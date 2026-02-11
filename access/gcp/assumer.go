package gcp

import (
	"context"
	"encoding/base64"
	"fmt"

	"golang.org/x/oauth2/google"
)

type Assumer struct {
	Credentials *google.Credentials
}

func AssumerFromBase64KeyFile(ctx context.Context, encoded string) (Assumer, error) {
	base64Decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return Assumer{}, fmt.Errorf("error decoding gcp service account key: %w", err)
	}

	creds, err := google.CredentialsFromJSONWithType(ctx, base64Decoded, google.ServiceAccount, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return Assumer{}, fmt.Errorf("error loading credentials from json: %w", err)
	}
	return Assumer{Credentials: creds}, nil
}
