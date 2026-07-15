module github.com/nullstone-io/infra-sdk

go 1.25.8

require (
	cloud.google.com/go/bigquery v1.79.0
	cloud.google.com/go/secretmanager v1.20.0
	github.com/aws/aws-sdk-go-v2 v1.42.1
	github.com/aws/aws-sdk-go-v2/credentials v1.19.29
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.41.1
	github.com/aws/aws-sdk-go-v2/service/apigatewayv2 v1.36.1
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.66.1
	github.com/aws/aws-sdk-go-v2/service/costexplorer v1.66.1
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.316.1
	github.com/aws/aws-sdk-go-v2/service/ecs v1.88.1
	github.com/aws/aws-sdk-go-v2/service/efs v1.43.1
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.55.1
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing v1.35.1
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.56.1
	github.com/aws/aws-sdk-go-v2/service/kafka v1.55.1
	github.com/aws/aws-sdk-go-v2/service/mq v1.38.0
	github.com/aws/aws-sdk-go-v2/service/opensearch v1.74.1
	github.com/aws/aws-sdk-go-v2/service/rds v1.120.1
	github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi v1.34.1
	github.com/aws/aws-sdk-go-v2/service/route53 v1.64.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.105.1
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.43.1
	github.com/aws/aws-sdk-go-v2/service/sns v1.41.1
	github.com/aws/aws-sdk-go-v2/service/sqs v1.45.1
	github.com/stretchr/testify v1.11.1
	golang.org/x/net v0.57.0
	golang.org/x/oauth2 v0.36.0
	google.golang.org/api v0.288.0
	google.golang.org/grpc v1.82.0
	gopkg.in/nullstone-io/go-api-client.v0 v0.0.0-20260713142524-4ef720e14350
)

require (
	cel.dev/expr v0.25.2 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.22.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/datacatalog v1.33.0 // indirect
	cloud.google.com/go/iam v1.12.0 // indirect
	cloud.google.com/go/monitoring v1.30.0 // indirect
	cloud.google.com/go/storage v1.63.1 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.34.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.58.0 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apache/arrow/go/v15 v15.0.2 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/apparentlymart/go-textseg/v17 v17.0.1 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.14 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.31 // indirect
	github.com/aws/smithy-go v1.27.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.18 // indirect
	github.com/googleapis/gax-go/v2 v2.23.0 // indirect
	github.com/hashicorp/hcl/v2 v2.24.0 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/klauspost/compress v1.19.0 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/nullstone-io/module v0.2.10 // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spiffe/go-spiffe/v2 v2.8.1 // indirect
	github.com/tmccombs/hcl2json v0.6.9 // indirect
	github.com/zclconf/go-cty v1.19.0 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.44.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.69.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/exp v0.0.0-20260709172345-9ea1abe57597 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/telemetry v0.0.0-20260710170516-c325552849a7 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	google.golang.org/genproto v0.0.0-20260713224248-f5fc221cf8c4 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260713224248-f5fc221cf8c4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260713224248-f5fc221cf8c4 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
