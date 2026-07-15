package rbac

import (
	"context"
	"fmt"
	"opertraitor/internal/ui"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Cache holds cluster-wide RBAC data to avoid excessive calls
type Cache struct {
	CRBs []rbacv1.ClusterRoleBinding
	RBs  map[string][]rbacv1.RoleBinding
}

func NewCache(ctx context.Context, client *kubernetes.Clientset) (*Cache, error) {
	ui.LogInfo("Initializing RBAC cache...")

	crbs, err := client.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list ClusterRoleBindings: %w", err)
	}

	rbsList, err := client.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list RoleBindings: %w", err)
	}

	rbMap := make(map[string][]rbacv1.RoleBinding)
	for _, rb := range rbsList.Items {
		rbMap[rb.Namespace] = append(rbMap[rb.Namespace], rb)
	}

	ui.LogSuccess(fmt.Sprintf("Cache built: Found %d ClusterRoleBindings and %d RoleBindings.", len(crbs.Items), len(rbsList.Items)))
	return &Cache{CRBs: crbs.Items, RBs: rbMap}, nil
}
