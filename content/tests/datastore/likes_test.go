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
	dsidx "github.com/panyam/goapplib/datastore"
	commonv1 "github.com/panyam/goapplib/content/gen/go/common/v1"
	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"github.com/panyam/goapplib/content/services/likes/backends"
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

var testKinds = []string{
	"Like",
	"LikeCounts",
	"ReactionType",
}


func isEmulatorAvailable() bool {
	return os.Getenv("DATASTORE_EMULATOR_HOST") != ""
}

func skipIfNoEmulator(t *testing.T) {
	if !isEmulatorAvailable() {
		t.Skip("Skipping: DATASTORE_EMULATOR_HOST not set. Run with Datastore emulator for integration tests.")
	}
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
func setupTestClient(t *testing.T) *datastore.Client {
	skipIfNoDatastore(t)

	ctx := context.Background()
	projectID := getProjectID()

	if isRealDatastoreConfigured() {
		return setupRealDatastoreClient(t, ctx, projectID)
	}

	// Emulator mode
	client, err := datastore.NewClient(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create Datastore client: %v", err)
	}

	t.Cleanup(func() {
		// Clean up test entities
		for _, kind := range testKinds {
			cleanupKind(ctx, client, kind)
		}
		client.Close()
	})

	return client
}

func setupRealDatastoreClient(t *testing.T, ctx context.Context, projectID string) *datastore.Client {
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

	// Ensure namespace is empty
	ensureNamespaceEmpty(t, ctx, client, namespace, testKinds)

	t.Cleanup(func() {
		cleanupNamespace(ctx, client, namespace)
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

func cleanupNamespace(ctx context.Context, client *datastore.Client, namespace string) error {
	for _, kind := range testKinds {
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
			if err := cleanupNamespace(ctx, client, namespace); err != nil {
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

// setupLikesService creates a likes service for testing.
func setupLikesService(t *testing.T) *backends.DatastoreLikesService {
	client := setupTestClient(t)
	namespace := getTestNamespace()
	ctx := context.Background()

	var service *backends.DatastoreLikesService
	var err error

	// For real Datastore, validate indexes
	if isRealDatastoreConfigured() && !*skipIndexValidation {
		service, err = backends.NewDatastoreLikesService(client, namespace, dsidx.WithValidation(ctx))
	} else {
		service, err = backends.NewDatastoreLikesService(client, namespace)
	}

	if err != nil {
		t.Fatalf("Failed to create likes service: %v", err)
	}

	return service
}

// TestLikesService_AddReaction tests adding reactions.
func TestLikesService_AddReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	resp, err := service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityType:   "post",
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	if resp.Like == nil {
		t.Fatal("Expected like to be returned")
	}
	if resp.Like.EntityType != "post" {
		t.Errorf("Expected entity_type=post, got %s", resp.Like.EntityType)
	}
	if resp.Like.ReactionType != "like" {
		t.Errorf("Expected reaction_type=like, got %s", resp.Like.ReactionType)
	}

	if resp.Counts == nil {
		t.Fatal("Expected counts to be returned")
	}
	if resp.Counts.TotalCount != 1 {
		t.Errorf("Expected total_count=1, got %d", resp.Counts.TotalCount)
	}
}

// TestLikesService_ToggleReaction tests toggling reactions.
func TestLikesService_ToggleReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Toggle on
	resp, err := service.ToggleReaction(ctx, &v1.ToggleReactionRequest{
		EntityType:   "post",
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("ToggleReaction failed: %v", err)
	}
	if !resp.Added {
		t.Error("Expected added=true")
	}

	// Toggle off
	resp, err = service.ToggleReaction(ctx, &v1.ToggleReactionRequest{
		EntityType:   "post",
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "like",
	})
	if err != nil {
		t.Fatalf("ToggleReaction failed: %v", err)
	}
	if resp.Added {
		t.Error("Expected added=false")
	}
}

// TestLikesService_GetUserReaction tests getting a user's reaction.
func TestLikesService_GetUserReaction(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Check before adding
	resp, err := service.GetUserReaction(ctx, &v1.GetUserReactionRequest{
		EntityType: "post",
		EntityId:   "post-1",
		UserId:     "user-1",
	})
	if err != nil {
		t.Fatalf("GetUserReaction failed: %v", err)
	}
	if resp.Like != nil {
		t.Error("Expected no reaction before adding")
	}

	// Add reaction
	_, err = service.AddReaction(ctx, &v1.AddReactionRequest{
		EntityType:   "post",
		EntityId:     "post-1",
		UserId:       "user-1",
		ReactionType: "love",
	})
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Check after adding
	resp, err = service.GetUserReaction(ctx, &v1.GetUserReactionRequest{
		EntityType: "post",
		EntityId:   "post-1",
		UserId:     "user-1",
	})
	if err != nil {
		t.Fatalf("GetUserReaction failed: %v", err)
	}
	if resp.Like == nil {
		t.Fatal("Expected reaction after adding")
	}
	if resp.Like.ReactionType != "love" {
		t.Errorf("Expected reaction_type=love, got %s", resp.Like.ReactionType)
	}
}

// TestLikesService_BatchGetLikeCounts tests batch getting counts.
func TestLikesService_BatchGetLikeCounts(t *testing.T) {
	service := setupLikesService(t)
	ctx := context.Background()

	// Add reactions to multiple entities
	entities := []struct {
		entityType string
		entityID   string
		count      int
	}{
		{"post", "post-1", 3},
		{"post", "post-2", 2},
	}

	for _, e := range entities {
		for i := 0; i < e.count; i++ {
			_, err := service.AddReaction(ctx, &v1.AddReactionRequest{
				EntityType:   e.entityType,
				EntityId:     e.entityID,
				UserId:       fmt.Sprintf("user-%s-%d", e.entityID, i),
				ReactionType: "like",
			})
			if err != nil {
				t.Fatalf("AddReaction failed: %v", err)
			}
		}
	}

	// Batch get counts
	resp, err := service.BatchGetLikeCounts(ctx, &v1.BatchGetLikeCountsRequest{
		Entities: []*commonv1.EntityRef{
			{EntityType: "post", EntityId: "post-1"},
			{EntityType: "post", EntityId: "post-2"},
		},
	})
	if err != nil {
		t.Fatalf("BatchGetLikeCounts failed: %v", err)
	}

	if counts, ok := resp.Counts["post:post-1"]; ok {
		if counts.TotalCount != 3 {
			t.Errorf("Expected post:post-1 count=3, got %d", counts.TotalCount)
		}
	} else {
		t.Error("Missing counts for post:post-1")
	}

	if counts, ok := resp.Counts["post:post-2"]; ok {
		if counts.TotalCount != 2 {
			t.Errorf("Expected post:post-2 count=2, got %d", counts.TotalCount)
		}
	} else {
		t.Error("Missing counts for post:post-2")
	}
}
