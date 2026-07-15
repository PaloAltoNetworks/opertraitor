package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// OperatorRbacInfo is one scanned operator with its RBAC rules and optional analysis.
type OperatorRbacInfo struct {
	ID           string       `dynamodbav:"ID" json:"id"`
	OperatorName string       `dynamodbav:"OperatorName" json:"operatorName"`
	Version      string       `dynamodbav:"Version" json:"version"`
	Timestamp    string       `dynamodbav:"Timestamp" json:"timestamp"`
	Metadata     Metadata     `dynamodbav:"Metadata" json:"metadata"`
	Rules        []RbacRule   `dynamodbav:"Rules" json:"rules"`
	Analysis     *LLMResponse `dynamodbav:"Analysis,omitempty" json:"analysis,omitempty"`
}

type Metadata struct {
	Provider       string       `dynamodbav:"Provider" json:"provider"`
	CatalogSource  string       `dynamodbav:"CatalogSource" json:"catalogSource,omitempty"`
	Maturity       string       `dynamodbav:"Maturity" json:"maturity,omitempty"`
	Description    string       `dynamodbav:"Description" json:"description,omitempty"`
	CreatedAt      time.Time    `dynamodbav:"CreatedAt" json:"createdAt,omitempty"`
	Categories     []string     `dynamodbav:"Categories" json:"categories,omitempty"`
	Keywords       []string     `dynamodbav:"Keywords" json:"keywords,omitempty"`
	Maintainers    []Maintainer `dynamodbav:"Maintainers" json:"maintainers,omitempty"`
	Links          []Link       `dynamodbav:"Links" json:"links,omitempty"`
	Capabilities   string       `dynamodbav:"Capabilities" json:"capabilities,omitempty"`
	MinKubeVersion string       `dynamodbav:"MinKubeVersion" json:"minKubeVersion,omitempty"`
	Repository     string       `dynamodbav:"Repository" json:"repository,omitempty"`
	Replaces       string       `dynamodbav:"Replaces" json:"replaces,omitempty"`
}

type Maintainer struct {
	Name  string `dynamodbav:"Name" json:"name"`
	Email string `dynamodbav:"Email" json:"email"`
}

type Link struct {
	Name string `dynamodbav:"Name" json:"name"`
	URL  string `dynamodbav:"URL" json:"url"`
}

type RbacRule struct {
	Scope          string   `dynamodbav:"Scope" json:"scope"`
	ServiceAccount string   `dynamodbav:"ServiceAccount" json:"serviceAccount"`
	APIGroups      []string `dynamodbav:"APIGroups" json:"apiGroups"`
	Resources      []string `dynamodbav:"Resources" json:"resources"`
	ResourceNames  []string `dynamodbav:"ResourceNames,omitempty" json:"resourceNames,omitempty"`
	Verbs          []string `dynamodbav:"Verbs" json:"verbs"`
}

// LLMResponse is the parsed JSON from the risk analysis prompt.
type LLMResponse struct {
	ArtifactName             string                     `json:"artifactName"`
	Version                  string                     `json:"version"`
	Summary                  string                     `json:"summary"`
	OverallRiskScore         int                        `json:"overall_risk_score"`
	RBACPermissionEvaluation []RBACPermissionEvaluation `json:"rbac_permission_evaluation"`
}

type RBACPermissionEvaluation struct {
	APIGroup       FlexibleStringArray `json:"api_group"`
	Resources      FlexibleStringArray `json:"resources"`
	Verbs          FlexibleStringArray `json:"verbs"`
	Scope          string              `json:"scope"`
	Permissiveness string              `json:"permissiveness"`
	Reasoning      string              `json:"reasoning"`
	RiskScore      int                 `json:"risk_score"`
}

type PullOptions struct {
	Timeout         time.Duration
	TotalLimitBytes int64
}

type FilterOptions struct {
	OperatorName string
	Version      string
}

func (f FilterOptions) Match(name, version string) bool {
	if f.OperatorName != "" {
		nameLower := strings.ToLower(name)
		filterLower := strings.ToLower(f.OperatorName)
		if nameLower != filterLower && !strings.Contains(nameLower, filterLower) {
			return false
		}
	}
	if f.Version != "" && version != f.Version {
		return false
	}
	return true
}

// NormalizeOperatorName provides a consistent casing for IDs and lookups.
func NormalizeOperatorName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// BuildDocID creates a stable document ID regardless of input casing.
func BuildDocID(operatorName, version string) string {
	return fmt.Sprintf("%s-%s", NormalizeOperatorName(operatorName), strings.TrimSpace(version))
}

// --- Custom Types for Robust Unmarshaling ---

// FlexibleStringArray unmarshals "foo" or ["foo", "bar"] into []string{"foo", "bar"}
type FlexibleStringArray []string

func (sa *FlexibleStringArray) UnmarshalJSON(data []byte) error {
	// Try unmarshalling as a single string first
	var singleString string
	if err := json.Unmarshal(data, &singleString); err == nil {
		*sa = []string{singleString}
		return nil
	}

	// If that fails, try unmarshalling as a standard string array
	var stringSlice []string
	if err := json.Unmarshal(data, &stringSlice); err != nil {
		return err
	}
	*sa = stringSlice
	return nil
}
