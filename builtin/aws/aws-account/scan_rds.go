package aws_account

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanRdsDatabases(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	rdsClient := rds.NewFromConfig(config)
	var resources []infra_sdk.ScanResource

	// Scan RDS instances (single instance databases)
	if err := scanRdsInstances(ctx, rdsClient, &resources); err != nil {
		return nil, fmt.Errorf("failed to scan RDS instances: %w", err)
	}

	// Scan RDS clusters (Aurora and Multi-AZ)
	if err := scanRdsClusters(ctx, rdsClient, &resources); err != nil {
		return nil, fmt.Errorf("failed to scan RDS clusters: %w", err)
	}

	return resources, nil
}

func scanRdsInstances(ctx context.Context, client *rds.Client, resources *[]infra_sdk.ScanResource) error {
	// Get all DB instances
	paginator := rds.NewDescribeDBInstancesPaginator(client, &rds.DescribeDBInstancesInput{})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, instance := range output.DBInstances {
			if instance.DBInstanceStatus == nil || *instance.DBInstanceStatus == "deleting" {
				continue
			}

			// Determine database type
			dbType, dbSubtype := getRdsDatabaseType(instance.Engine, instance.EngineVersion)
			if dbType == "" {
				continue // Skip unsupported database types
			}

			// Extract tags
			tags := make(map[string]string)
			if instance.TagList != nil {
				for _, tag := range instance.TagList {
					if tag.Key != nil && tag.Value != nil {
						tags[*tag.Key] = *tag.Value
					}
				}
			}

			// Get the instance name (use DBInstanceIdentifier if Name tag is not present)
			name := instance.DBInstanceIdentifier
			if nameTag, exists := tags["Name"]; exists && nameTag != "" {
				name = &nameTag
			}

			*resources = append(*resources, infra_sdk.ScanResource{
				UniqueId: *instance.DBInstanceArn,
				Name:     *name,
				Taxonomy: infra_sdk.ResourceTaxonomy{
					Category:    types.CategoryDatastore,
					Subcategory: "",
					Platform:    dbType,
					Subplatform: dbSubtype,
				},
				Attributes: map[string]any{
					"identifier":        instance.DBInstanceIdentifier,
					"engine":            instance.Engine,
					"engine_version":    instance.EngineVersion,
					"instance_class":    instance.DBInstanceClass,
					"storage_type":      instance.StorageType,
					"allocated_storage": instance.AllocatedStorage,
					"multi_az":          instance.MultiAZ,
					"status":            instance.DBInstanceStatus,
					"endpoint":          instance.Endpoint,
					"tags":              tags,
				},
			})
		}
	}

	return nil
}

func scanRdsClusters(ctx context.Context, client *rds.Client, resources *[]infra_sdk.ScanResource) error {
	// Get all DB clusters
	paginator := rds.NewDescribeDBClustersPaginator(client, &rds.DescribeDBClustersInput{})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, cluster := range output.DBClusters {
			if cluster.Status == nil || *cluster.Status == "deleting" {
				continue
			}

			// Determine database type
			dbType, dbSubtype := getRdsDatabaseType(cluster.Engine, cluster.EngineVersion)
			if dbType == "" {
				continue // Skip unsupported database types
			}

			// Extract tags
			tags := make(map[string]string)
			if cluster.TagList != nil {
				for _, tag := range cluster.TagList {
					if tag.Key != nil && tag.Value != nil {
						tags[*tag.Key] = *tag.Value
					}
				}
			}

			// Get the cluster name (use DBClusterIdentifier if Name tag is not present)
			name := cluster.DBClusterIdentifier
			if nameTag, exists := tags["Name"]; exists && nameTag != "" {
				name = &nameTag
			}

			// Get the writer instance
			var writerEndpoint string
			if cluster.Endpoint != nil {
				writerEndpoint = *cluster.Endpoint
			}

			// Get reader endpoints
			readerEndpoints := make([]string, 0)
			if cluster.ReaderEndpoint != nil {
				readerEndpoints = append(readerEndpoints, *cluster.ReaderEndpoint)
			}

			*resources = append(*resources, infra_sdk.ScanResource{
				UniqueId: *cluster.DBClusterArn,
				Name:     *name,
				Taxonomy: infra_sdk.ResourceTaxonomy{
					Category:    types.CategoryDatastore,
					Subcategory: "",
					Platform:    dbType,
					Subplatform: dbSubtype,
				},
				Attributes: map[string]any{
					"identifier":          cluster.DBClusterIdentifier,
					"engine":              cluster.Engine,
					"engine_version":      cluster.EngineVersion,
					"engine_mode":         cluster.EngineMode,
					"status":              cluster.Status,
					"writer_endpoint":     writerEndpoint,
					"reader_endpoints":    readerEndpoints,
					"multi_az":            cluster.MultiAZ,
					"storage_encrypted":   cluster.StorageEncrypted,
					"database_name":       cluster.DatabaseName,
					"backup_retention":    cluster.BackupRetentionPeriod,
					"cluster_members":     cluster.DBClusterMembers,
					"vpc_security_groups": cluster.VpcSecurityGroups,
					"tags":                tags,
				},
			})
		}
	}

	return nil
}

// getRdsDatabaseType determines the database type and subtype based on engine and version
func getRdsDatabaseType(engine, version *string) (string, string) {
	if engine == nil {
		return "", ""
	}

	switch *engine {
	case "mysql":
		return "mysql", "rds"
	case "postgres":
		return "postgres", "rds"
	case "aurora-mysql", "aurora":
		return "mysql", "aurora"
	case "aurora-postgresql":
		return "postgres", "aurora"
	default:
		return "", ""
	}
}
