// Package datastore provides integration tests for content services using Google Cloud Datastore.
package datastore

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/datastore"
	"github.com/panyam/goapplib/content/services/likes/backends"
	tagsbackends "github.com/panyam/goapplib/content/services/tags/backends"
	dsidx "github.com/panyam/goapplib/datastore"
	"google.golang.org/api/option"
)

// Environment variables for Datastore configuration:
//
// Emulator mode:
//   - DATASTORE_EMULATOR_HOST: Emulator host (e.g., "localhost:8081")
//   - DATASTORE_PROJECT_ID: Project ID for emulator (default: "test-project")
//
// Real Datastore mode:
//   - DATASTORE_PROJECT_ID: GCP project ID (required)
//   - DATASTORE_CREDENTIALS_FILE: Path to service account JSON (optional, uses ADC if not set)
//   - DATASTORE_TEST_NAMESPACE: Namespace for test entities (required for real Datastore)

const (
	envDatastoreCredentials = "DATASTORE_CREDENTIALS_FILE"
	envDatastoreNamespace   = "DATASTORE_TEST_NAMESPACE"
)

var forceDeleteNamespace = flag.Bool("force-delete-ns", false,
	"Force delete existing entities in test namespace before running tests")

var skipIndexValidation = flag.Bool("skip-index-validation", false,
	"Skip index validation (useful for emulator which doesn't require indexes)")

var generateIndexesOnly = flag.Bool("generate-indexes", false,
	"Generate index YAML files and exit without running tests")

// TestMain validates indexes once before running any tests.
func TestMain(m *testing.M) {
	flag.Parse()

	// Generate index files mode
	if *generateIndexesOnly {
		if err := generateAllIndexFiles(); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating index files: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Skip validation for emulator or if explicitly disabled
	if !isEmulatorAvailable() && isRealDatastoreConfigured() && !*skipIndexValidation {
		if err := validateAllIndexes(); err != nil {
			fmt.Fprintf(os.Stderr, "\n%s\n", err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

// validateAllIndexes validates indexes for all services once at startup.
func validateAllIndexes() error {
	ctx := context.Background()
	projectID := getProjectID()
	namespace := getTestNamespace()

	client, err := createDatastoreClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to create Datastore client: %w", err)
	}
	defer client.Close()

	var allErrors []string

	// Validate likes service
	likesService, err := backends.NewDatastoreLikesService(client, namespace, dsidx.WithValidation(ctx))
	if err != nil {
		allErrors = append(allErrors, err.Error())
	}
	_ = likesService // unused, just for validation

	// Validate tags service
	tagsService, err := tagsbackends.NewDatastoreTagsService(client, namespace, dsidx.WithValidation(ctx))
	if err != nil {
		allErrors = append(allErrors, err.Error())
	}
	_ = tagsService // unused, just for validation

	if len(allErrors) > 0 {
		return fmt.Errorf("%s", strings.Join(allErrors, "\n\n"))
	}

	return nil
}

// generateAllIndexFiles generates index YAML files for all services.
// Uses the actual service index definitions.
// Requires DATASTORE_CREDENTIALS_FILE or default credentials to be set.
func generateAllIndexFiles() error {
	ctx := context.Background()
	projectID := getProjectID()

	client, err := createDatastoreClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to create Datastore client: %w", err)
	}
	defer client.Close()

	// Create service instances (no validation - just for index generation)
	likesService, err := backends.NewDatastoreLikesService(client, "")
	if err != nil {
		return fmt.Errorf("failed to create likes service: %w", err)
	}
	tagsService, err := tagsbackends.NewDatastoreTagsService(client, "")
	if err != nil {
		return fmt.Errorf("failed to create tags service: %w", err)
	}

	// Write per-service index files
	if err := likesService.WriteIndexFile("likes_index.yaml"); err != nil {
		return fmt.Errorf("failed to write likes_index.yaml: %w", err)
	}
	fmt.Printf("Written: likes_index.yaml\n")
	fmt.Printf("  Deploy: cp likes_index.yaml /tmp/index.yaml && gcloud --project=%s datastore indexes create /tmp/index.yaml\n\n", projectID)

	if err := tagsService.WriteIndexFile("tags_index.yaml"); err != nil {
		return fmt.Errorf("failed to write tags_index.yaml: %w", err)
	}
	fmt.Printf("Written: tags_index.yaml\n")
	fmt.Printf("  Deploy: cp tags_index.yaml /tmp/index.yaml && gcloud --project=%s datastore indexes create /tmp/index.yaml\n\n", projectID)

	// Write combined index.yaml
	if err := dsidx.WriteCombinedIndexFile("index.yaml", likesService, tagsService); err != nil {
		return fmt.Errorf("failed to write index.yaml: %w", err)
	}
	fmt.Printf("Written: index.yaml (combined)\n")
	fmt.Printf("  Deploy: gcloud --project=%s datastore indexes create index.yaml\n\n", projectID)

	fmt.Println("Index generation complete!")
	fmt.Println("\nTo deploy all indexes at once, run:")
	fmt.Printf("  gcloud --project=%s datastore indexes create index.yaml\n", projectID)
	return nil
}

// createDatastoreClient creates a Datastore client for the given project.
func createDatastoreClient(ctx context.Context, projectID string) (*datastore.Client, error) {
	credFile := os.Getenv(envDatastoreCredentials)
	if credFile != "" {
		credFile = expandPath(credFile)
		return datastore.NewClient(ctx, projectID, option.WithCredentialsFile(credFile))
	}
	return datastore.NewClient(ctx, projectID)
}

func isEmulatorAvailable() bool {
	return os.Getenv("DATASTORE_EMULATOR_HOST") != ""
}

func getProjectID() string {
	projectID := os.Getenv("DATASTORE_PROJECT_ID")
	if projectID == "" {
		projectID = "test-project"
	}
	return projectID
}

func getTestNamespace() string {
	return os.Getenv(envDatastoreNamespace)
}

func isRealDatastoreConfigured() bool {
	return os.Getenv(envDatastoreNamespace) != "" && !isEmulatorAvailable()
}

func skipIfNoDatastore(t *testing.T) {
	if !isEmulatorAvailable() && !isRealDatastoreConfigured() {
		t.Skip("Skipping: No Datastore configured. Set DATASTORE_EMULATOR_HOST for emulator or DATASTORE_TEST_NAMESPACE for real Datastore.")
	}
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// setupTestClient creates a Datastore client for testing.
func setupTestClient(t *testing.T, kinds []string) *datastore.Client {
	skipIfNoDatastore(t)

	ctx := context.Background()
	projectID := getProjectID()

	if isRealDatastoreConfigured() {
		return setupRealDatastoreClient(t, ctx, projectID, kinds)
	}

	// Emulator mode
	client, err := datastore.NewClient(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create Datastore client: %v", err)
	}

	t.Cleanup(func() {
		// Clean up test entities
		for _, kind := range kinds {
			cleanupKind(ctx, client, kind)
		}
		client.Close()
	})

	return client
}

func setupRealDatastoreClient(t *testing.T, ctx context.Context, projectID string, kinds []string) *datastore.Client {
	namespace := getTestNamespace()

	if projectID == "" || projectID == "test-project" {
		t.Fatal("DATASTORE_PROJECT_ID must be set to a real GCP project ID for real Datastore tests")
	}

	var client *datastore.Client
	var err error

	credFile := os.Getenv(envDatastoreCredentials)
	if credFile != "" {
		credFile = expandPath(credFile)
		if _, statErr := os.Stat(credFile); os.IsNotExist(statErr) {
			t.Fatalf("Credentials file does not exist: %s", credFile)
		}
		client, err = datastore.NewClient(ctx, projectID, option.WithCredentialsFile(credFile))
	} else {
		client, err = datastore.NewClient(ctx, projectID)
	}

	if err != nil {
		t.Fatalf("Failed to create Datastore client: %v", err)
	}

	// Ensure namespace is empty for the specific kinds we're testing
	ensureNamespaceEmpty(t, ctx, client, namespace, kinds)

	t.Cleanup(func() {
		cleanupNamespaceKinds(ctx, client, namespace, kinds)
		client.Close()
	})

	return client
}

func cleanupKind(ctx context.Context, client *datastore.Client, kind string) error {
	q := datastore.NewQuery(kind).KeysOnly()
	keys, err := client.GetAll(ctx, q, nil)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return client.DeleteMulti(ctx, keys)
	}
	return nil
}

func cleanupKindInNamespace(ctx context.Context, client *datastore.Client, kind, namespace string) error {
	q := datastore.NewQuery(kind).Namespace(namespace).KeysOnly()
	keys, err := client.GetAll(ctx, q, nil)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return client.DeleteMulti(ctx, keys)
	}
	return nil
}

func namespaceHasEntities(ctx context.Context, client *datastore.Client, namespace string, kinds []string) (bool, error) {
	for _, kind := range kinds {
		q := datastore.NewQuery(kind).Namespace(namespace).KeysOnly().Limit(1)
		keys, err := client.GetAll(ctx, q, nil)
		if err != nil {
			return false, fmt.Errorf("failed to query kind %q in namespace %q: %w", kind, namespace, err)
		}
		if len(keys) > 0 {
			return true, nil
		}
	}
	return false, nil
}

func cleanupNamespaceKinds(ctx context.Context, client *datastore.Client, namespace string, kinds []string) error {
	for _, kind := range kinds {
		if err := cleanupKindInNamespace(ctx, client, kind, namespace); err != nil {
			return fmt.Errorf("failed to cleanup kind %q: %w", kind, err)
		}
	}
	return nil
}

func ensureNamespaceEmpty(t *testing.T, ctx context.Context, client *datastore.Client, namespace string, kinds []string) {
	hasEntities, err := namespaceHasEntities(ctx, client, namespace, kinds)
	if err != nil {
		t.Fatalf("Failed to check namespace for existing entities: %v", err)
	}

	if hasEntities {
		if *forceDeleteNamespace {
			t.Logf("Namespace %q has existing entities. Deleting due to -force-delete-ns flag...", namespace)
			if err := cleanupNamespaceKinds(ctx, client, namespace, kinds); err != nil {
				t.Fatalf("Failed to cleanup existing entities in namespace %q: %v", namespace, err)
			}
			t.Logf("Cleanup complete.")
		} else {
			t.Fatalf(`Namespace %q already has entities for test kinds %v.

This is a safety check to prevent accidental data loss. To proceed:

Option 1: Use a different namespace
  export DATASTORE_TEST_NAMESPACE=my-unique-test-namespace

Option 2: Force delete existing entities
  go test ./tests/datastore/... -args -force-delete-ns

Option 3: Manually delete entities in the namespace first
`, namespace, kinds)
		}
	}
}
