package scanner

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"opertraitor/internal/models"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// allowedRegistries are the only registries we will pull bundle images from by default.
// This blocks SSRF via attacker-controlled image references in
// PackageManifests (e.g. internal hosts, cloud metadata endpoints).
var allowedRegistries = map[string]bool{
	"quay.io":                     true,
	"registry.redhat.io":          true,
	"registry.connect.redhat.com": true,
}

// Hard limits to prevent decompression bombs from malicious bundle images.
const (
	maxBundleBytes   = 256 * 1024 * 1024 // 256 MiB cap on total extracted bytes
	maxBundleObjects = 10000             // cap on YAML/JSON documents per bundle
)

// FetchBundleYAMLs pulls a bundle image and returns the K8s objects and image creation time from it.
func FetchBundleYAMLs(ctx context.Context, imageName string, opts models.PullOptions) ([]*unstructured.Unstructured, time.Time, error) {
	ref, err := name.ParseReference(imageName)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("invalid image name: %w", err)
	}

	// Registry allow-list: only pull from a known set of public catalogs.
	if !allowedRegistries[ref.Context().RegistryStr()] {
		return nil, time.Time{}, fmt.Errorf("refusing to pull from disallowed registry %q", ref.Context().RegistryStr())
	}

	// Use the docker keychain so users who hit anonymous rate limits can
	// authenticate via `docker login`.
	img, err := remote.Image(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to pull image: %w", err)
	}

	// --- EXTRACT IMAGE METADATA ---
	var imageTime time.Time
	// Use the image config's "created" timestamp as a fallback date.
	configFile, err := img.ConfigFile()
	if err == nil && !configFile.Created.Time.IsZero() {
		imageTime = configFile.Created.Time
	}

	// Flatten all layers into a single tar stream, capped to maxBundleBytes
	// so a malicious image cannot exhaust memory.
	fs := mutate.Extract(img)
	defer fs.Close()

	limited := io.LimitReader(fs, maxBundleBytes)
	tr := tar.NewReader(limited)
	var objects []*unstructured.Unstructured

	for {
		if len(objects) >= maxBundleObjects {
			return nil, time.Time{}, fmt.Errorf("bundle exceeds %d object limit", maxBundleObjects)
		}

		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, time.Time{}, err
		}

		// Only process regular files. Skips symlinks/hardlinks/devices that
		// could otherwise be used in path-traversal style abuse.
		// (TypeReg covers both old "regular" and "regular-A" tar entries
		// since Go 1.11; TypeRegA is deprecated.)
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Skip non-manifest files (YAML/JSON).
		if strings.HasSuffix(header.Name, ".yaml") || strings.HasSuffix(header.Name, ".json") || strings.HasSuffix(header.Name, ".yml") {
			decoder := yaml.NewYAMLOrJSONDecoder(tr, 4096)
			for {
				if len(objects) >= maxBundleObjects {
					return nil, time.Time{}, fmt.Errorf("bundle exceeds %d object limit", maxBundleObjects)
				}
				var raw map[string]interface{}
				if err := decoder.Decode(&raw); err != nil {
					if err == io.EOF {
						break
					}
					// Skip malformed chunks
					continue
				}
				if len(raw) == 0 {
					continue
				}

				obj := &unstructured.Unstructured{Object: raw}
				objects = append(objects, obj)
			}
		}
	}

	return objects, imageTime, nil
}

// FindBundleImage picks the bundle image ref from the default channel.
func FindBundleImage(channels []interface{}, defaultChan string) string {
	for _, ch := range channels {
		chMap, ok := ch.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(chMap, "name")
		if defaultChan != "" && name != defaultChan {
			continue
		}
		csvDesc, _, _ := unstructured.NestedMap(chMap, "currentCSVDesc")

		// Check RelatedImages from "quay.io/operatorhubio/"
		related, _, _ := unstructured.NestedSlice(csvDesc, "relatedImages")
		for _, img := range related {
			var imgStr string
			if iMap, ok := img.(map[string]interface{}); ok {
				imgStr, _, _ = unstructured.NestedString(iMap, "image")
			} else if iStr, ok := img.(string); ok {
				imgStr = iStr
			}

			if imgStr != "" {
				if strings.Contains(imgStr, "quay.io/operatorhubio/") {
					return imgStr
				}
			}
		}

		// Check Annotations - "containerImage"
		if ann, _, _ := unstructured.NestedStringMap(csvDesc, "annotations"); ann != nil {
			if img, ok := ann["containerImage"]; ok && img != "" {
				return img
			}
		}

		// If we matched the default channel and found nothing, stop looking further
		if defaultChan != "" && name == defaultChan {
			break
		}
	}
	return ""
}
