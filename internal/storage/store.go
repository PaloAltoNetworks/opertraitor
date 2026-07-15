package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opertraitor/internal/models"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esutil"
)

// Store is the persistence interface for scan results.
type Store interface {
	Save(info models.OperatorRbacInfo) error
	Get(operatorName, version string) (models.OperatorRbacInfo, error)
	Scan(ctx context.Context, filter models.FilterOptions) ([]models.OperatorRbacInfo, error)
	SaveLLMAnalysis(docID string, analysis models.LLMResponse) error
}

// --- Print Store ---

func NewPrintStore() Store { return &printStore{} }

type printStore struct{}

func (p *printStore) Save(info models.OperatorRbacInfo) error {
	info.Timestamp = time.Now().UTC().Format(time.RFC3339)
	info.ID = models.BuildDocID(info.OperatorName, info.Version)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Operator: %s\n", info.OperatorName)
	fmt.Printf("Version:  %s\n", info.Version)
	if info.Metadata.Description != "" {
		fmt.Printf("Description: %s\n", info.Metadata.Description)
	}
	if info.Metadata.Provider != "" {
		fmt.Printf("Provider: %s\n", info.Metadata.Provider)
	}
	fmt.Printf("Timestamp: %s\n", info.Timestamp)
	if len(info.Rules) > 0 {
		fmt.Println("\nRBAC Rules:")
		for i, rule := range info.Rules {
			fmt.Printf("  [%d] Scope: %s, ServiceAccount: %s\n", i+1, rule.Scope, rule.ServiceAccount)
			fmt.Printf("      APIGroups: %v\n", rule.APIGroups)
			fmt.Printf("      Resources: %v\n", rule.Resources)
			fmt.Printf("      Verbs: %v\n", rule.Verbs)
			if len(rule.ResourceNames) > 0 {
				fmt.Printf("      ResourceNames: %v\n", rule.ResourceNames)
			}
		}
	} else {
		fmt.Println("No RBAC rules found")
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	return nil
}

func (p *printStore) Get(name, version string) (models.OperatorRbacInfo, error) {
	return models.OperatorRbacInfo{}, fmt.Errorf("get not supported for print output")
}

func (p *printStore) Scan(ctx context.Context, filter models.FilterOptions) ([]models.OperatorRbacInfo, error) {
	return nil, fmt.Errorf("scan not supported for print output (write-only)")
}

func (p *printStore) SaveLLMAnalysis(docID string, analysis models.LLMResponse) error {
	fmt.Printf("\n--- Analysis Result (%s) ---\n", docID)
	fmt.Printf("Risk Score: %d/10\n", analysis.OverallRiskScore)
	fmt.Printf("Summary: %s\n", analysis.Summary)
	fmt.Println("------------------------------")
	return nil
}

// --- File Store ---

// NewFileStore opens or creates the JSON file at filePath.
func NewFileStore(filePath string) (Store, error) {
	cleaned := filepath.Clean(filePath)

	if !filepath.IsAbs(cleaned) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve working directory: %w", err)
		}
		abs := filepath.Join(cwd, cleaned)
		// Ensure the resolved absolute path is still under cwd.
		rel, err := filepath.Rel(cwd, abs)
		if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
			return nil, fmt.Errorf("file path %q escapes the working directory", filePath)
		}
		cleaned = abs
	}

	dir := filepath.Dir(cleaned)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}
	return &fileStore{filePath: cleaned, mu: &sync.Mutex{}}, nil
}

type fileStore struct {
	filePath string
	mu       *sync.Mutex
}

// acquireFileLock creates an exclusive lock sidecar (`<path>.lock`) so two
// processes pointing at the same output file cannot interleave writes and corrupt the JSON
func acquireFileLock(targetPath string) (*os.File, error) {
	lockPath := targetPath + ".lock"
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("failed to acquire lock: %w", err)
		}
		// Steal a stale lock left by a crashed process.
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > 60*time.Second {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for lock %s", lockPath)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func releaseFileLock(f *os.File) {
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: failed to remove lock file %s: %v", name, err)
	}
}

func writeFileWithFallback(targetPath string, data []byte) error {
	tmpFile := targetPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	// Attempt atomic rename.
	var renameErr error
	for i := 0; i < 5; i++ {
		renameErr = os.Rename(tmpFile, targetPath)
		if renameErr == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Last-resort fallback: direct write. Not strictly atomic, but the
	// surrounding file lock guarantees no other process is writing.
	if writeErr := os.WriteFile(targetPath, data, 0600); writeErr != nil {
		if rmErr := os.Remove(tmpFile); rmErr != nil {
			log.Printf("warning: failed to remove temp file %s: %v", tmpFile, rmErr)
		}
		return fmt.Errorf("failed to rename temp file: %w; fallback write failed: %v", renameErr, writeErr)
	}
	if rmErr := os.Remove(tmpFile); rmErr != nil {
		log.Printf("warning: failed to remove temp file %s: %v", tmpFile, rmErr)
	}
	return nil
}

func (f *fileStore) Save(info models.OperatorRbacInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	lock, err := acquireFileLock(f.filePath)
	if err != nil {
		return err
	}
	defer releaseFileLock(lock)
	info.Timestamp = time.Now().UTC().Format(time.RFC3339)
	info.ID = models.BuildDocID(info.OperatorName, info.Version)
	var existingData []models.OperatorRbacInfo
	if _, err := os.Stat(f.filePath); err == nil {
		fileData, err := os.ReadFile(f.filePath)
		if err == nil && len(fileData) > 0 {
			if err := json.Unmarshal(fileData, &existingData); err != nil {
				existingData = []models.OperatorRbacInfo{}
			}
		}
	}
	found := false
	for i, existing := range existingData {
		if existing.ID == info.ID {
			existingData[i] = info
			found = true
			break
		}
	}
	if !found {
		existingData = append(existingData, info)
	}
	data, err := json.MarshalIndent(existingData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	// Use atomic write when possible, fall back when the target file is locked.
	return writeFileWithFallback(f.filePath, data)
}

func (f *fileStore) Get(name, version string) (models.OperatorRbacInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fileData, err := os.ReadFile(f.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return models.OperatorRbacInfo{}, fmt.Errorf("file not found: %s", f.filePath)
		}
		return models.OperatorRbacInfo{}, fmt.Errorf("failed to read file: %w", err)
	}
	if len(fileData) == 0 {
		return models.OperatorRbacInfo{}, fmt.Errorf("file is empty")
	}
	var data []models.OperatorRbacInfo
	if err := json.Unmarshal(fileData, &data); err != nil {
		scanner := bufio.NewScanner(strings.NewReader(string(fileData)))
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var info models.OperatorRbacInfo
			if err := json.Unmarshal(line, &info); err == nil {
				data = append(data, info)
			}
		}
	}
	targetID := models.BuildDocID(name, version)
	for _, info := range data {
		if info.ID == targetID {
			return info, nil
		}
	}
	return models.OperatorRbacInfo{}, fmt.Errorf("operator %s version %s not found", name, version)
}

func (f *fileStore) Scan(ctx context.Context, filter models.FilterOptions) ([]models.OperatorRbacInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fileData, err := os.ReadFile(f.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.OperatorRbacInfo{}, nil
		}
		return nil, err
	}
	if len(fileData) == 0 {
		return []models.OperatorRbacInfo{}, nil
	}
	var allData []models.OperatorRbacInfo
	if err := json.Unmarshal(fileData, &allData); err != nil {
		scanner := bufio.NewScanner(strings.NewReader(string(fileData)))
		for scanner.Scan() {
			var info models.OperatorRbacInfo
			if err := json.Unmarshal(scanner.Bytes(), &info); err == nil {
				allData = append(allData, info)
			}
		}
	}
	var filtered []models.OperatorRbacInfo
	for _, item := range allData {
		if filter.Match(item.OperatorName, item.Version) {
			filtered = append(filtered, item)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		scoreI := 0
		if filtered[i].Analysis != nil {
			scoreI = filtered[i].Analysis.OverallRiskScore
		}
		scoreJ := 0
		if filtered[j].Analysis != nil {
			scoreJ = filtered[j].Analysis.OverallRiskScore
		}
		return scoreI > scoreJ // Descending Order (High to Low)
	})

	return filtered, nil
}

func (f *fileStore) SaveLLMAnalysis(docID string, analysis models.LLMResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	lock, err := acquireFileLock(f.filePath)
	if err != nil {
		return err
	}
	defer releaseFileLock(lock)
	fileData, err := os.ReadFile(f.filePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var existingData []models.OperatorRbacInfo
	if len(fileData) > 0 {
		if err := json.Unmarshal(fileData, &existingData); err != nil {
			return fmt.Errorf("corrupted data file, cannot unmarshal: %w", err)
		}
	}
	found := false
	for i, item := range existingData {
		currentID := item.ID
		if currentID == "" {
			currentID = models.BuildDocID(item.OperatorName, item.Version)
		}
		if currentID == docID {
			existingData[i].Analysis = &analysis
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("record with ID %s not found in file to update analysis", docID)
	}
	data, err := json.MarshalIndent(existingData, "", "  ")
	if err != nil {
		return err
	}
	// Use atomic write when possible, fall back when the target file is locked.
	return writeFileWithFallback(f.filePath, data)
}

// --- Elastic Store ---

type ElasticStore struct {
	client *elasticsearch.Client
	index  string
}

func NewElasticStore(cloudID, apiKey, index string) (*ElasticStore, error) {
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		CloudID: cloudID,
		APIKey:  apiKey,
	})
	if err != nil {
		return nil, err
	}
	return &ElasticStore{client: client, index: index}, nil
}

func (e *ElasticStore) Save(info models.OperatorRbacInfo) error {
	info.Timestamp = time.Now().UTC().Format(time.RFC3339)
	info.ID = models.BuildDocID(info.OperatorName, info.Version)
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	res, err := e.client.Index(
		e.index,
		bytes.NewReader(data),
		e.client.Index.WithDocumentID(info.ID),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch index error: %s", res.String())
	}
	return nil
}

func (e *ElasticStore) Get(name, version string) (models.OperatorRbacInfo, error) {
	docID := models.BuildDocID(name, version)

	res, err := e.client.Get(e.index, docID)
	if err != nil {
		return models.OperatorRbacInfo{}, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return models.OperatorRbacInfo{}, fmt.Errorf("elastic error: %s", res.Status())
	}

	var r map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return models.OperatorRbacInfo{}, err
	}

	source, ok := r["_source"]
	if !ok {
		return models.OperatorRbacInfo{}, fmt.Errorf("document not found")
	}

	sourceBytes, _ := json.Marshal(source)
	var item models.OperatorRbacInfo
	if err := json.Unmarshal(sourceBytes, &item); err != nil {
		return models.OperatorRbacInfo{}, err
	}

	return item, nil
}

func (e *ElasticStore) Scan(ctx context.Context, filter models.FilterOptions) ([]models.OperatorRbacInfo, error) {
	var items []models.OperatorRbacInfo
	query := map[string]interface{}{
		"query": map[string]interface{}{"match_all": map[string]interface{}{}},
		"size":  1000,
		"sort": []map[string]interface{}{
			{
				"analysis.overall_risk_score": map[string]interface{}{
					"order":   "desc",
					"missing": "_last", // Put items without analysis at the bottom
				},
			},
		},
	}

	if filter.OperatorName != "" {
		query["query"] = map[string]interface{}{
			"term": map[string]interface{}{"operatorName.keyword": filter.OperatorName},
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, err
	}

	res, err := e.client.Search(
		e.client.Search.WithContext(ctx),
		e.client.Search.WithIndex(e.index),
		e.client.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		var errMap map[string]interface{}
		if json.NewDecoder(res.Body).Decode(&errMap) == nil {
			if reason, ok := errMap["error"].(map[string]interface{}); ok {
				return nil, fmt.Errorf("elastic search error: %v", reason["reason"])
			}
		}
		return nil, fmt.Errorf("elastic search error: %s", res.Status())
	}

	var r map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return nil, err
	}

	// Guard against nil hits
	hitsRoot, ok := r["hits"].(map[string]interface{})
	if !ok {
		return items, nil
	}

	hitsList, ok := hitsRoot["hits"].([]interface{})
	if !ok {
		return items, nil
	}

	for _, hit := range hitsList {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}
		source, ok := hitMap["_source"]
		if !ok {
			continue
		}

		sourceBytes, _ := json.Marshal(source)
		var item models.OperatorRbacInfo
		if err := json.Unmarshal(sourceBytes, &item); err == nil {
			items = append(items, item)
		}
	}

	return items, nil
}

func (e *ElasticStore) SaveLLMAnalysis(docID string, analysis models.LLMResponse) error {
	updateBody := map[string]interface{}{
		"doc": map[string]interface{}{
			"analysis": analysis,
		},
		"doc_as_upsert": true,
	}
	body := esutil.NewJSONReader(&updateBody)
	res, err := e.client.Update(e.index, docID, body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("update error: %s", res.String())
	}
	return nil
}
