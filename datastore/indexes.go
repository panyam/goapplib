// Package datastore provides shared utilities for Google Cloud Datastore backends.
package datastore

import (
	"context"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/datastore"
)

// IndexProperty defines a property in a composite index.
type IndexProperty struct {
	Name      string
	Direction string // "asc", "desc", or "" for default ascending
}

// DatastoreIndex defines a composite index for Datastore.
type DatastoreIndex struct {
	Kind       string
	Properties []IndexProperty
}

// IndexProvider is implemented by Datastore services that require composite indexes.
type IndexProvider interface {
	// ServiceName returns the name of the service (e.g., "likes", "tags").
	ServiceName() string

	// RequiredIndexes returns the composite indexes required by this service.
	RequiredIndexes() []DatastoreIndex

	// TestQueries returns queries that exercise each required index.
	// These are used to validate that indexes exist.
	TestQueries() []*datastore.Query
}

// IndexValidationError contains information about missing indexes.
type IndexValidationError struct {
	ServiceName    string
	MissingIndexes []DatastoreIndex
	Errors         []error
}

func (e *IndexValidationError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Missing %d Datastore index(es) for %s service:\n\n", len(e.MissingIndexes), e.ServiceName))

	for i, idx := range e.MissingIndexes {
		sb.WriteString(fmt.Sprintf("  %d. Kind: %s, Properties: ", i+1, idx.Kind))
		props := make([]string, len(idx.Properties))
		for j, p := range idx.Properties {
			if p.Direction == "desc" {
				props[j] = p.Name + " (desc)"
			} else {
				props[j] = p.Name
			}
		}
		sb.WriteString(strings.Join(props, ", "))
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("\nTo fix, run:\n  %s\n", PrintIndexCommand(e.ServiceName)))
	return sb.String()
}

// ValidateIndexes checks if all required indexes exist by running test queries.
// Returns nil if all indexes exist, or an IndexValidationError with details about missing indexes.
func ValidateIndexes(ctx context.Context, client *datastore.Client, namespace string, provider IndexProvider) error {
	queries := provider.TestQueries()
	indexes := provider.RequiredIndexes()

	if len(queries) != len(indexes) {
		return fmt.Errorf("mismatch: %d test queries but %d indexes defined", len(queries), len(indexes))
	}

	var missingIndexes []DatastoreIndex
	var errors []error

	for i, q := range queries {
		if namespace != "" {
			q = q.Namespace(namespace)
		}

		// Run a keys-only query with limit 1 to test if index exists
		q = q.KeysOnly().Limit(1)

		_, err := client.GetAll(ctx, q, nil)
		if err != nil {
			// Check if it's a missing index error
			if strings.Contains(err.Error(), "no matching index") ||
				strings.Contains(err.Error(), "recommended index is") {
				missingIndexes = append(missingIndexes, indexes[i])
				errors = append(errors, err)
			}
			// Ignore other errors (empty results, etc.)
		}
	}

	if len(missingIndexes) > 0 {
		return &IndexValidationError{
			ServiceName:    provider.ServiceName(),
			MissingIndexes: missingIndexes,
			Errors:         errors,
		}
	}

	return nil
}

// IndexesToYAML converts indexes to YAML format suitable for index.yaml.
func IndexesToYAML(serviceName string, indexes []DatastoreIndex) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Datastore indexes for %s service\n", serviceName))
	sb.WriteString("# Deploy with: gcloud datastore indexes create " + serviceName + "_index.yaml\n\n")
	sb.WriteString("indexes:\n")

	for _, idx := range indexes {
		sb.WriteString(fmt.Sprintf("\n- kind: %s\n", idx.Kind))
		sb.WriteString("  properties:\n")
		for _, prop := range idx.Properties {
			sb.WriteString(fmt.Sprintf("  - name: %s\n", prop.Name))
			if prop.Direction == "desc" {
				sb.WriteString("    direction: desc\n")
			}
		}
	}

	return sb.String()
}

// WriteIndexFile writes the indexes to a YAML file.
func WriteIndexFile(path string, serviceName string, indexes []DatastoreIndex) error {
	yaml := IndexesToYAML(serviceName, indexes)
	return os.WriteFile(path, []byte(yaml), 0644)
}

// PrintIndexCommand returns the gcloud command to create indexes.
// Includes --project flag if DATASTORE_PROJECT_ID is set.
// Note: Datastore indexes are project-wide (not namespace-specific).
// The file is copied to /tmp/index.yaml because gcloud requires the file to be named index.yaml.
func PrintIndexCommand(serviceName string) string {
	projectID := os.Getenv("DATASTORE_PROJECT_ID")
	indexFile := IndexFileName(serviceName)
	if projectID != "" {
		return fmt.Sprintf("cp %s /tmp/index.yaml && gcloud --project=%s datastore indexes create /tmp/index.yaml", indexFile, projectID)
	}
	return fmt.Sprintf("cp %s /tmp/index.yaml && gcloud datastore indexes create /tmp/index.yaml", indexFile)
}

// ValidateMultipleServices validates indexes for multiple services and collects all errors.
// Returns a combined error message if any services have missing indexes.
func ValidateMultipleServices(ctx context.Context, client *datastore.Client, namespace string, providers ...IndexProvider) error {
	var allErrors []*IndexValidationError

	for _, provider := range providers {
		err := ValidateIndexes(ctx, client, namespace, provider)
		if err != nil {
			if validErr, ok := err.(*IndexValidationError); ok {
				allErrors = append(allErrors, validErr)
			}
		}
	}

	if len(allErrors) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Missing Datastore indexes for %d service(s):\n\n", len(allErrors)))

	for _, err := range allErrors {
		sb.WriteString("=" + strings.Repeat("=", 60) + "\n")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}

	sb.WriteString("=" + strings.Repeat("=", 60) + "\n")
	sb.WriteString("\nTo fix all, run these commands:\n")
	for _, err := range allErrors {
		sb.WriteString(fmt.Sprintf("  %s\n", PrintIndexCommand(err.ServiceName)))
	}
	sb.WriteString("\nNote: Indexes are project-wide (not namespace-specific).\n")
	sb.WriteString("Wait for indexes to build (check GCP Console > Datastore > Indexes).\n")

	return fmt.Errorf("%s", sb.String())
}

// WriteAllIndexFiles writes index files for multiple services.
func WriteAllIndexFiles(outputDir string, providers ...IndexProvider) error {
	for _, provider := range providers {
		filename := fmt.Sprintf("%s/%s_index.yaml", outputDir, provider.ServiceName())
		if err := WriteIndexFile(filename, provider.ServiceName(), provider.RequiredIndexes()); err != nil {
			return fmt.Errorf("failed to write %s: %w", filename, err)
		}
		fmt.Printf("Wrote %s\n", filename)
		fmt.Printf("  Deploy with: %s\n\n", PrintIndexCommand(provider.ServiceName()))
	}
	return nil
}

// IndexFileName returns the default index file name for a service.
func IndexFileName(serviceName string) string {
	return serviceName + "_index.yaml"
}

// CombinedIndexFileName is the standard name expected by gcloud.
const CombinedIndexFileName = "index.yaml"

// CombineIndexesToYAML combines indexes from multiple providers into one YAML.
func CombineIndexesToYAML(providers ...IndexProvider) string {
	var sb strings.Builder
	sb.WriteString("# Combined Datastore indexes\n")
	sb.WriteString("# Deploy with: gcloud datastore indexes create index.yaml\n\n")
	sb.WriteString("indexes:\n")

	for _, provider := range providers {
		sb.WriteString(fmt.Sprintf("\n# %s service\n", provider.ServiceName()))
		for _, idx := range provider.RequiredIndexes() {
			sb.WriteString(fmt.Sprintf("\n- kind: %s\n", idx.Kind))
			sb.WriteString("  properties:\n")
			for _, prop := range idx.Properties {
				sb.WriteString(fmt.Sprintf("  - name: %s\n", prop.Name))
				if prop.Direction == "desc" {
					sb.WriteString("    direction: desc\n")
				}
			}
		}
	}

	return sb.String()
}

// WriteCombinedIndexFile writes a combined index.yaml for multiple services.
func WriteCombinedIndexFile(path string, providers ...IndexProvider) error {
	yaml := CombineIndexesToYAML(providers...)
	return os.WriteFile(path, []byte(yaml), 0644)
}

// ValidateAndWriteIndexes validates indexes and writes the index file if validation fails.
// Returns a user-friendly error with deployment instructions.
func ValidateAndWriteIndexes(ctx context.Context, client *datastore.Client, namespace string, provider IndexProvider) error {
	err := ValidateIndexes(ctx, client, namespace, provider)
	if err == nil {
		return nil
	}

	indexFile := IndexFileName(provider.ServiceName())

	// Try to write the index file
	var writeMsg string
	if writeErr := WriteIndexFile(indexFile, provider.ServiceName(), provider.RequiredIndexes()); writeErr == nil {
		writeMsg = fmt.Sprintf("Index file written: %s\n\n", indexFile)
	} else {
		writeMsg = fmt.Sprintf("(Failed to write index file: %v)\n\n", writeErr)
	}

	return fmt.Errorf(`%s
======================================================================
%sTo deploy the required indexes, run:

  %s

Note: Indexes are project-wide (not namespace-specific).
Wait for indexes to build (check status in GCP Console > Datastore > Indexes),
then restart your application.
======================================================================
`, err.Error(), writeMsg, PrintIndexCommand(provider.ServiceName()))
}

// ServiceOptions configures a Datastore service.
type ServiceOptions struct {
	// ValidateCtx, if non-nil, triggers index validation during construction.
	// If validation fails, the constructor returns an error with deployment instructions.
	ValidateCtx context.Context

	// KindNames allows overriding default kind (table) names.
	// Keys are default kind names, values are custom names.
	// Example: {"Tag": "MyApp_Tag", "EntityTag": "MyApp_EntityTag"}
	KindNames map[string]string
}

// ServiceOption is a functional option for configuring Datastore services.
type ServiceOption func(*ServiceOptions)

// WithValidation enables index validation during service construction.
// If indexes are missing, the constructor returns an error with deployment instructions.
func WithValidation(ctx context.Context) ServiceOption {
	return func(opts *ServiceOptions) {
		opts.ValidateCtx = ctx
	}
}

// WithKindNames allows overriding default Datastore kind (table) names.
// Useful for multi-tenant scenarios or avoiding naming conflicts.
// Example: WithKindNames(map[string]string{"Tag": "MyApp_Tag"})
func WithKindNames(names map[string]string) ServiceOption {
	return func(opts *ServiceOptions) {
		opts.KindNames = names
	}
}

// ApplyOptions applies functional options and returns the resulting config.
func ApplyOptions(options ...ServiceOption) *ServiceOptions {
	opts := &ServiceOptions{}
	for _, opt := range options {
		opt(opts)
	}
	return opts
}
