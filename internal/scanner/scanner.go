package scanner

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"opertraitor/internal/models"
	"opertraitor/internal/rbac"
	"opertraitor/internal/storage"
	"opertraitor/internal/ui"
	"opertraitor/internal/worker"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Workers Config
const (
	WorkersInstalled = 8
	WorkersAvailable = 20
	JobTimeout       = 30 * time.Minute
)

// --- Scanners ---

func ScanInstalled(ctx context.Context, clientset *kubernetes.Clientset, dynClient dynamic.Interface, store storage.Store, filter models.FilterOptions) {
	ui.PrintHeader("Scanning INSTALLED Operators")

	rbacCache, err := rbac.NewCache(ctx, clientset)
	if err != nil {
		ui.LogError(fmt.Sprintf("Could not build RBAC cache: %v", err))
		return
	}

	gvr := schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions"}
	csvs, err := dynClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		ui.LogError(fmt.Sprintf("Error listing CSVs. Is OLM installed? Error: %v", err))
		return
	}

	total := len(csvs.Items)
	var processed atomic.Int32
	showProgress := filter.OperatorName == ""

	worker.ProcessQueue(ctx, total, WorkersInstalled, JobTimeout, func(jobs chan<- interface{}) {
		for _, csv := range csvs.Items {
			jobs <- csv
		}
	}, func(ctx context.Context, obj interface{}) {
		if showProgress {
			current := processed.Add(1)
			ui.PrintProgress(int(current), total)
		}

		csv := obj.(unstructured.Unstructured)
		processSingleInstalled(ctx, clientset, csv, rbacCache, store, filter)
	})

	if showProgress {
		fmt.Println()
	}
	ui.LogSuccess("Scan of INSTALLED operators finished successfully.")
}

func ScanAvailable(ctx context.Context, dynClient dynamic.Interface, store storage.Store, pullOpts models.PullOptions, filter models.FilterOptions) {
	ui.PrintHeader("Scanning AVAILABLE Operators (Catalog)")

	gvr := schema.GroupVersionResource{Group: "packages.operators.coreos.com", Version: "v1", Resource: "packagemanifests"}
	pms, err := dynClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		ui.LogError(fmt.Sprintf("Error listing PackageManifests: %v", err))
		return
	}

	total := len(pms.Items)
	var processed atomic.Int32
	showProgress := filter.OperatorName == ""

	worker.ProcessQueue(ctx, total, WorkersAvailable, JobTimeout, func(jobs chan<- interface{}) {
		for _, pm := range pms.Items {
			jobs <- pm
		}
	}, func(ctx context.Context, obj interface{}) {
		if showProgress {
			current := processed.Add(1)
			ui.PrintProgress(int(current), total)
		}

		pm := obj.(unstructured.Unstructured)
		processSingleAvailable(ctx, pm, store, pullOpts, filter)
	})

	if showProgress {
		fmt.Println()
	}
	ui.LogSuccess("Scan of AVAILABLE operators finished successfully.")
}

// --- Processors ---

func processSingleInstalled(ctx context.Context, client *kubernetes.Clientset, csv unstructured.Unstructured, cache *rbac.Cache, store storage.Store, filter models.FilterOptions) {
	name := csv.GetName()
	ns := csv.GetNamespace()

	var csvSpec struct {
		DisplayName string `json:"displayName"`
		Version     string `json:"version"`
		Install     struct {
			Spec struct {
				Deployments []struct {
					Spec struct {
						Template struct {
							Spec struct {
								ServiceAccountName string `json:"serviceAccountName"`
							} `json:"spec"`
						} `json:"template"`
					} `json:"spec"`
				} `json:"deployments"`
			} `json:"spec"`
		} `json:"install"`
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(csv.Object["spec"].(map[string]interface{}), &csvSpec); err != nil {
		ui.LogWarn(fmt.Sprintf("Skipping %s/%s: Malformed CSV spec", ns, name))
		return
	}

	if !filter.Match(csvSpec.DisplayName, csvSpec.Version) {
		return
	}

	sas := make(map[string]bool)
	for _, dep := range csvSpec.Install.Spec.Deployments {
		if sa := dep.Spec.Template.Spec.ServiceAccountName; sa != "" {
			sas[sa] = true
		}
	}

	if len(sas) == 0 {
		return
	}

	info := models.OperatorRbacInfo{
		OperatorName: csvSpec.DisplayName,
		Version:      csvSpec.Version,
	}

	info.Metadata = extractMetadata(csv.Object)

	if labels, _, _ := unstructured.NestedStringMap(csv.Object, "metadata", "labels"); labels != nil {
		if cs, ok := labels["operators.coreos.com/catalog-source"]; ok {
			info.Metadata.CatalogSource = cs
		}
	}

	rbac.ExtractRulesFromCSVSpec(csv.Object, &info, sas)
	rbac.ExtractLiveRules(ctx, client, ns, sas, cache, &info)

	if err := store.Save(info); err != nil {
		ui.LogError(fmt.Sprintf("Database save failed for '%s': %v", csvSpec.DisplayName, err))
	} else {
		ui.LogSuccess(fmt.Sprintf("Processed: %s %s(%s)%s - %d rules", csvSpec.DisplayName, ui.ColorGray, csvSpec.Version, ui.ColorReset, len(info.Rules)))
	}
}

func processSingleAvailable(ctx context.Context, pm unstructured.Unstructured, store storage.Store, opts models.PullOptions, filter models.FilterOptions) {
	pkgName, _, _ := unstructured.NestedString(pm.Object, "status", "packageName")
	catalogSource, _, _ := unstructured.NestedString(pm.Object, "status", "catalogSource")

	matched := filter.Match(pkgName, "")

	channels, _, _ := unstructured.NestedSlice(pm.Object, "status", "channels")
	defaultChan, _, _ := unstructured.NestedString(pm.Object, "status", "defaultChannel")

	if !matched {
		for _, ch := range channels {
			chMap, ok := ch.(map[string]interface{})
			if !ok {
				continue
			}
			csvDesc, _, _ := unstructured.NestedMap(chMap, "currentCSVDesc")
			displayName, _, _ := unstructured.NestedString(csvDesc, "displayName")
			if filter.Match(displayName, "") {
				matched = true
				break
			}
		}
	}

	if !matched {
		return
	}

	bundleImage := FindBundleImage(channels, defaultChan)
	if bundleImage == "" {
		return
	}

	objs, imageTime, err := FetchBundleYAMLs(ctx, bundleImage, opts)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			ui.LogWarn(fmt.Sprintf("[TIMEOUT] Skipped '%s': Took too long to fetch", pkgName))
		} else {
			ui.LogWarn(fmt.Sprintf("Fetch failed for '%s': %v", pkgName, err))
		}
		return
	}

	info := models.OperatorRbacInfo{OperatorName: pkgName}
	analyzeBundleObjects(objs, &info)

	// Fallback to imageTime if Metadata.CreatedAt is still empty
	if info.Metadata.CreatedAt.IsZero() && !imageTime.IsZero() {
		info.Metadata.CreatedAt = imageTime
	}

	if info.Metadata.CatalogSource == "" {
		info.Metadata.CatalogSource = catalogSource
	}

	if !filter.Match(info.OperatorName, info.Version) {
		return
	}

	if info.Version != "" {
		if err := store.Save(info); err != nil {
			ui.LogError(fmt.Sprintf("Database save failed for '%s': %v", pkgName, err))
		} else {
			ui.LogSuccess(fmt.Sprintf("Processed: %s %s(%s)%s", pkgName, ui.ColorGray, info.Version, ui.ColorReset))
		}
	}
}

// --- Analysis Helpers ---

func analyzeBundleObjects(objs []*unstructured.Unstructured, info *models.OperatorRbacInfo) {
	roles := make(map[string]*rbacv1.Role)
	clusterRoles := make(map[string]*rbacv1.ClusterRole)
	var csvObj *unstructured.Unstructured

	for _, obj := range objs {
		switch obj.GetKind() {
		case "ClusterServiceVersion":
			csvObj = obj
			info.Version, _, _ = unstructured.NestedString(obj.Object, "spec", "version")

			meta := extractMetadata(obj.Object)

			info.Metadata.Description = meta.Description
			info.Metadata.Provider = meta.Provider
			info.Metadata.Maturity = meta.Maturity
			info.Metadata.Categories = meta.Categories

			if !meta.CreatedAt.IsZero() {
				info.Metadata.CreatedAt = meta.CreatedAt
			}
		case "Role":
			var r rbacv1.Role
			_ = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &r)
			roles[obj.GetName()] = &r
		case "ClusterRole":
			var r rbacv1.ClusterRole
			_ = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &r)
			clusterRoles[obj.GetName()] = &r
		}
	}

	if csvObj != nil {
		rbac.ExtractRulesFromCSVSpec(csvObj.Object, info, nil)
	}

	if len(info.Rules) == 0 {
		for _, r := range roles {
			for _, rule := range r.Rules {
				info.Rules = append(info.Rules, rbac.ConvertRule(rule, "NamespaceScoped", "AllSAs"))
			}
		}
		for _, r := range clusterRoles {
			for _, rule := range r.Rules {
				info.Rules = append(info.Rules, rbac.ConvertRule(rule, "ClusterWide", "AllSAs"))
			}
		}
	}
}

// extractMetadata helps parse provider, maturity, description, CreatedAt AND Categories
func extractMetadata(csv map[string]interface{}) models.Metadata {
	var meta models.Metadata
	meta.Description, _, _ = unstructured.NestedString(csv, "spec", "description")
	meta.Maturity, _, _ = unstructured.NestedString(csv, "spec", "maturity")

	// Provider
	providerMap, found, _ := unstructured.NestedMap(csv, "spec", "provider")
	if found {
		if name, ok := providerMap["name"].(string); ok {
			meta.Provider = name
		}
	} else {
		if name, found, _ := unstructured.NestedString(csv, "spec", "provider"); found {
			meta.Provider = name
		}
	}

	// CreatedAt
	if ts, found, _ := unstructured.NestedString(csv, "metadata", "creationTimestamp"); found && ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			meta.CreatedAt = t
		}
	}
	if meta.CreatedAt.IsZero() {
		annotations, _, _ := unstructured.NestedStringMap(csv, "metadata", "annotations")
		if val, ok := annotations["createdAt"]; ok {
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				meta.CreatedAt = t
			}
		} else if val, ok := annotations["olm.operatorframework.io/createdAt"]; ok {
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				meta.CreatedAt = t
			}
		}
	}

	// Categories
	// OLM typically stores categories as a comma-separated string in annotations
	annotations, _, _ := unstructured.NestedStringMap(csv, "metadata", "annotations")
	if annotations != nil {
		if cats, ok := annotations["categories"]; ok && cats != "" {
			parts := strings.Split(cats, ",")
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					meta.Categories = append(meta.Categories, trimmed)
				}
			}
		}
	}

	return meta
}
