package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/supermemory-native/supermemory-native/internal/api"
	"github.com/supermemory-native/supermemory-native/internal/db"
	"github.com/supermemory-native/supermemory-native/internal/embedding"
	"github.com/supermemory-native/supermemory-native/internal/memory"
	"github.com/supermemory-native/supermemory-native/internal/vault"
)

func main() {
	log.Println("Starting Supermemory-Native Daemon...")

	// 1. Resolve Port
	port := os.Getenv("PORT")
	if port == "" {
		port = "6767"
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
	if err := os.MkdirAll(dbDir, 0755); err != nil {
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
	case "bifrost", "openai":
		model := os.Getenv("EMBEDDING_MODEL")
		if model == "" {
			model = "Sense/Qwen3-Embedding-0.6B-GGUF"
		}
		baseURL := os.Getenv("OPENAI_BASE_URL")
		if baseURL == "" {
			baseURL = "https://bifrost.xairo.ipv64.net/v1"
		}
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("SUPERMEMORY_API_KEY")
		}
		log.Printf("Using OpenAI/Bifrost Embedding Provider with model: %s, baseURL: %s", model, baseURL)
		provider = embedding.NewOpenAIProvider(model, baseURL, apiKey)
	default:
		log.Println("Using Gemini Cloud Embedding Provider")
		provider = embedding.NewGeminiProvider(apiKey)
	}
	engine := memory.NewEngine(sdb, provider, v)
	handler := api.NewHandler(engine)

	// 5. Start Server
	addr := fmt.Sprintf("0.0.0.0:%s", port)
	log.Printf("Supermemory-Native Server listening on http://%s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("failed to start http server: %v", err)
	}
}
