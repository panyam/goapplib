package gcs_test

import (
	"context"

	"github.com/panyam/goapplib/storage/connections"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func googleDefault(ctx context.Context) (oauth2.TokenSource, error) {
	c, err := google.FindDefaultCredentials(ctx, connections.GCSReadScope)
	if err != nil {
		return nil, err
	}
	return c.TokenSource, nil
}
