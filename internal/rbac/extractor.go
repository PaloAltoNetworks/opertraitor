package rbac

import (
	"context"
	"opertraitor/internal/models"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

type permissionDef struct {
	ServiceAccountName string              `json:"serviceAccountName"`
	Rules              []rbacv1.PolicyRule `json:"rules"`
}

// ExtractRulesFromCSVSpec parses the "install" section of a CSV.
func ExtractRulesFromCSVSpec(csvObj map[string]interface{}, info *models.OperatorRbacInfo, limitToSAs map[string]bool) {
	var installSpec struct {
		Spec struct {
			Permissions        []permissionDef `json:"permissions"`
			ClusterPermissions []permissionDef `json:"clusterPermissions"`
		} `json:"spec"`
	}

	installMap, found, _ := unstructured.NestedMap(csvObj, "spec", "install")
	if !found {
		return
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(installMap, &installSpec); err != nil {
		return
	}

	appendRules := func(defs []permissionDef, scope string) {
		for _, def := range defs {
			sa := def.ServiceAccountName
			if limitToSAs != nil && sa != "" && !limitToSAs[sa] {
				continue
			}
			if sa == "" {
				sa = "AllSAs"
			}

			for _, r := range def.Rules {
				info.Rules = append(info.Rules, ConvertRule(r, scope, sa))
			}
		}
	}

	appendRules(installSpec.Spec.ClusterPermissions, "ClusterWide")
	appendRules(installSpec.Spec.Permissions, "NamespaceScoped")
}

// ExtractLiveRules matches SAs to Bindings to Roles in the live cluster.
func ExtractLiveRules(ctx context.Context, client *kubernetes.Clientset, ns string, sas map[string]bool, cache *Cache, info *models.OperatorRbacInfo) {
	for _, crb := range cache.CRBs {
		for _, sub := range crb.Subjects {
			if sub.Kind == "ServiceAccount" && sub.Namespace == ns && sas[sub.Name] {
				addClusterRoleRules(ctx, client, crb.RoleRef.Name, sub.Name, "ClusterWide", info)
			}
		}
	}

	if rbs, ok := cache.RBs[ns]; ok {
		for _, rb := range rbs {
			for _, sub := range rb.Subjects {
				if sub.Kind == "ServiceAccount" && sas[sub.Name] {
					if rb.RoleRef.Kind == "ClusterRole" {
						addClusterRoleRules(ctx, client, rb.RoleRef.Name, sub.Name, "NamespaceScoped", info)
					} else {
						addRoleRules(ctx, client, ns, rb.RoleRef.Name, sub.Name, "NamespaceScoped", info)
					}
				}
			}
		}
	}
}

func addClusterRoleRules(ctx context.Context, client *kubernetes.Clientset, roleName, sa, scope string, info *models.OperatorRbacInfo) {
	cr, err := client.RbacV1().ClusterRoles().Get(ctx, roleName, metav1.GetOptions{})
	if err == nil {
		for _, r := range cr.Rules {
			info.Rules = append(info.Rules, ConvertRule(r, scope, sa))
		}
	}
}

func addRoleRules(ctx context.Context, client *kubernetes.Clientset, ns, roleName, sa, scope string, info *models.OperatorRbacInfo) {
	role, err := client.RbacV1().Roles(ns).Get(ctx, roleName, metav1.GetOptions{})
	if err == nil {
		for _, r := range role.Rules {
			info.Rules = append(info.Rules, ConvertRule(r, scope, sa))
		}
	}
}

func ConvertRule(r rbacv1.PolicyRule, scope, sa string) models.RbacRule {
	return models.RbacRule{
		Scope:          scope,
		ServiceAccount: sa,
		APIGroups:      r.APIGroups,
		Resources:      r.Resources,
		Verbs:          r.Verbs,
		ResourceNames:  r.ResourceNames,
	}
}
