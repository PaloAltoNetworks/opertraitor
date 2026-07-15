package main

import (
	"context"
	"flag"
	"log"
	"opertraitor/internal/analyzer"
	"opertraitor/internal/models"
	"opertraitor/internal/scanner"
	"opertraitor/internal/storage"
	"opertraitor/internal/web"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	// --- Flags ---
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "Path to kubeconfig")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "Path to kubeconfig")
	}

	mode := flag.String("mode", "all", "Operation mode: 'scan', 'analyze', 'all', or 'web'.")
	scope := flag.String("scope", "all", "Scan scope: 'installed', 'available', 'all'.")
	output := flag.String("output", "", "Output storage: 'file' or 'elastic'.")
	filePath := flag.String("file-path", "opertraitor-output.json", "Path to output file.")

	// Web Flags
	webPort := flag.String("port", "8080", "Port for web UI (only used if mode=web)")
	webAddr := flag.String("address", "127.0.0.1", "Bind address for web UI (use 0.0.0.0 to allow external access, requires --allow-public-bind)")
	allowPublicBind := flag.Bool("allow-public-bind", false, "Required to bind the web UI to a non-loopback address")

	// API keys are intentionally not accepted as flags:
	esIndex := flag.String("es-index", "opertraitor-rbac", "Elastic index name")
	azModel := flag.String("az-model", "gpt-5.4", "Azure OpenAI Model")
	azTemp := flag.Float64("az-temp", 0.7, "LLM Temperature")
	azWorkers := flag.Int("az-workers", 12, "Workers")
	operatorName := flag.String("operator-name", "", "Filter by name")
	operatorVersion := flag.String("operator-version", "", "Filter by version")

	flag.Parse()

	// --- Resolve API keys from environment variables only ---
	resolveESCloudID := os.Getenv("ES_CLOUD_ID")
	resolveESAPIKey := os.Getenv("ES_API_KEY")
	resolveAZAPIKey := os.Getenv("AZ_API_KEY")
	resolveAZEndpoint := os.Getenv("AZ_ENDPOINT")

	// --- Init Storage ---
	var store storage.Store
	var err error

	if *output == "elastic" {
		if resolveESCloudID == "" || resolveESAPIKey == "" {
			log.Fatalf("Error: ES_CLOUD_ID and ES_API_KEY environment variables are required when output=elastic")
		}
		store, err = storage.NewElasticStore(resolveESCloudID, resolveESAPIKey, *esIndex)
	} else if *output == "file" {
		store, err = storage.NewFileStore(*filePath)
	} else if *mode == "web" {
		log.Println("Web mode: Defaulting to file storage at opertraitor-output.json")
		store, err = storage.NewFileStore(*filePath)
	} else {
		store = storage.NewPrintStore()
	}

	if err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}

	// --- WEB MODE ---
	if *mode == "web" {
		// Refuse to expose the unauthenticated UI on the network unless
		// the operator explicitly opts in. Loopback is allowed by default.
		if *webAddr != "127.0.0.1" && *webAddr != "localhost" && *webAddr != "::1" && !*allowPublicBind {
			log.Fatalf("Refusing to bind to %q without --allow-public-bind. The web UI has no authentication; only expose it on networks you trust.", *webAddr)
		}
		server := web.NewServer(store, *webAddr, *webPort)
		server.Start()
		return
	}

	// --- SCAN PHASE ---
	ctx := context.Background()
	filter := models.FilterOptions{OperatorName: *operatorName, Version: *operatorVersion}
	isScanMode := *mode == "scan" || *mode == "all" || *mode == "installed" || *mode == "available"

	if isScanMode {
		currentScope := *scope
		if *mode == "installed" {
			currentScope = "installed"
		}
		if *mode == "available" {
			currentScope = "available"
		}

		pullOpts := models.PullOptions{Timeout: 60 * time.Second}
		var clientset *kubernetes.Clientset
		var dynamicClient dynamic.Interface

		config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			log.Fatalf("Error building kubeconfig: %v", err)
		}

		if currentScope == "installed" || currentScope == "all" {
			clientset, err = kubernetes.NewForConfig(config)
			if err != nil {
				log.Fatalf("Error creating K8s client: %v", err)
			}
		}
		dynamicClient, err = dynamic.NewForConfig(config)
		if err != nil {
			log.Fatalf("Error creating dynamic client: %v", err)
		}

		if currentScope == "installed" || currentScope == "all" {
			scanner.ScanInstalled(ctx, clientset, dynamicClient, store, filter)
		}
		if currentScope == "available" || currentScope == "all" {
			scanner.ScanAvailable(ctx, dynamicClient, store, pullOpts, filter)
		}
	}

	// --- ANALYZE PHASE ---
	if *mode == "analyze" || *mode == "all" {
		if *output == "" && *mode != "all" {
			log.Fatalf("Error: --mode=analyze requires persistent storage.")
		}
		if resolveAZAPIKey == "" || resolveAZEndpoint == "" {
			if *mode == "all" {
				return
			}
			log.Fatalf("Error: AZ_API_KEY and AZ_ENDPOINT environment variables are required")
		}
		azClient, err := analyzer.NewClient(resolveAZAPIKey, resolveAZEndpoint)
		if err != nil {
			log.Fatalf("Failed to init Azure client: %v", err)
		}

		cfg := analyzer.Config{Model: *azModel, Temperature: float32(*azTemp), MaxTokens: 20000, Workers: *azWorkers}
		analyzer.RunAnalysis(ctx, azClient, store, filter, cfg)
	}
}
