package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/api"
	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/embedding"
	"github.com/supermemory-native/supermemory-native/internal/memory"
	"github.com/supermemory-native/supermemory-native/internal/vault"
)

func main() {
	log.Println("Starting Supermemory-Native Daemon...")

	// 1. Resolve Port and bind host. Loopback by default: exposing the daemon
	// to the LAN (or the world) is an explicit opt-in via HOST.
	port := os.Getenv("PORT")
	if port == "" {
		port = "6767"
	}
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	// 2. Resolve API Key
	apiKey := os.Getenv("SUPERMEMORY_API_KEY")
	if apiKey == "" {
		// Fallback: check GEMINI_API_KEY
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		log.Println("WARNING: neither SUPERMEMORY_API_KEY nor GEMINI_API_KEY is set. Cloud embeddings will fail.")
	}

	// 3. Resolve Database and Vault Path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("failed to get user home directory: %v", err)
	}

	dbDir := filepath.Join(homeDir, ".supermemory")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		log.Fatalf("failed to create database directory: %v", err)
	}

	dbPath := filepath.Join(dbDir, "memory_native.db")
	log.Printf("Initializing SQLite Database at: %s", dbPath)

	sdb, err := db.NewSqliteDB(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize SQLite: %v", err)
	}
	defer sdb.Close()

	// Initialize Vault directory
	vaultDir := filepath.Join(dbDir, "vault")
	v, err := vault.NewVault(vaultDir)
	if err != nil {
		log.Fatalf("failed to initialize vault: %v", err)
	}
	log.Printf("Initializing physical OKF Vault at: %s", vaultDir)

	// 3.5. Automatic Database-to-File Migration for Legacy Records
	unmigrated, err := sdb.GetUnmigratedMemories()
	if err != nil {
		log.Printf("WARNING: failed to check for unmigrated database records: %v", err)
	} else if len(unmigrated) > 0 {
		log.Printf("Found %d legacy unmigrated database records. Migrating to physical OKF Vault...", len(unmigrated))
		migratedCount := 0
		for _, m := range unmigrated {
			filePath, err := v.WriteMemoryWithTime(m.ID, m.Content, m.ContainerTag, m.CreatedAt)
			if err != nil {
				log.Printf("ERROR: failed to migrate memory %s: %v", m.ID, err)
				continue
			}
			if err := sdb.UpdateFilePath(m.ID, filePath); err != nil {
				log.Printf("ERROR: failed to update database file_path for %s: %v", m.ID, err)
				continue
			}
			migratedCount++
		}
		log.Printf("Successfully migrated %d/%d legacy records to physical files!", migratedCount, len(unmigrated))
	}

	// 4. Initialize Core Engine
	var provider embedding.EmbeddingProvider
	providerType := os.Getenv("EMBEDDING_PROVIDER")
	switch providerType {
	case "ollama":
		model := os.Getenv("EMBEDDING_MODEL")
		if model == "" {
			model = "nomic-embed-text"
		}
		log.Printf("Using Ollama Embedding Provider with model: %s", model)
		provider = embedding.NewOllamaProvider(model)
	case "openai":
		model := os.Getenv("EMBEDDING_MODEL")
		baseURL := os.Getenv("OPENAI_BASE_URL")
		apiKey := os.Getenv("OPENAI_API_KEY")
		log.Printf("Using OpenAI Embedding Provider with model: %s, baseURL: %s", model, baseURL)
		provider = embedding.NewOpenAIProvider(model, baseURL, apiKey)
	default:
		log.Println("Using Gemini Cloud Embedding Provider")
		provider = embedding.NewGeminiProvider(apiKey)
	}
	engine := memory.NewEngine(sdb, provider, v)
	handler := api.NewHandler(engine, port)

	// 5. Start Server
	addr := fmt.Sprintf("%s:%s", host, port)
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		if os.Getenv("SUPERMEMORY_API_KEY") == "" {
			log.Println("WARNING: listening on a non-loopback address without SUPERMEMORY_API_KEY — any host that can reach this port has full read/write access.")
		} else {
			log.Println("NOTE: non-loopback clients must authenticate with SUPERMEMORY_API_KEY; the bundled dashboard cannot send it from remote browsers.")
		}
	}
	log.Printf("Supermemory-Native Server listening on http://%s", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
		// Bounds against Slowloris-style and stalled connections.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("failed to start http server: %v", err)
	}
}
