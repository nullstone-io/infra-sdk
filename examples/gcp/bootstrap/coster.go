package bootstrap

import (
	"context"
	"os"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	gcp_project "github.com/nullstone-io/infra-sdk/builtin/gcp/gcp-project"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func NewCoster() (gcp_project.Coster, error) {
	return gcp_project.Coster{
		Accessor: GcpBillingAccessor{
			ProjectId: os.Getenv("BOOTSTRAP_GCP_PROJECT_ID"),
			Dataset:   os.Getenv("BOOTSTRAP_GCP_BILLING_DATASET"),
			Table:     os.Getenv("BOOTSTRAP_GCP_BILLING_TABLE"),
		},
	}, nil
}

var _ infra_sdk.GcpBillingAccessor = GcpBillingAccessor{}

type GcpBillingAccessor struct {
	ProjectId string
	Dataset   string
	Table     string
}

func (a GcpBillingAccessor) GetTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/bigquery.readonly")
	if err != nil {
		return nil, err
	}
	return creds.TokenSource, nil
}

func (a GcpBillingAccessor) GcpProjectId() string {
	return a.ProjectId
}

func (a GcpBillingAccessor) BillingDataset() string {
	return a.Dataset
}

func (a GcpBillingAccessor) BillingTable() string {
	return a.Table
}
