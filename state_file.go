package infra_sdk

import (
	"encoding/json"
	"fmt"
)

type StateFile struct {
	Version          int                        `json:"version"`
	TerraformVersion string                     `json:"terraform_version"`
	Serial           uint64                     `json:"serial"`
	Lineage          string                     `json:"lineage"`
	Outputs          map[string]StateFileOutput `json:"outputs"`
	Resources        []StateFileResource        `json:"resources"`
	CheckResults     []StateFileCheckResult     `json:"check_results"`
}

type StateFileOutput struct {
	Value     json.RawMessage `json:"value"`
	Type      json.RawMessage `json:"type"`
	Sensitive bool            `json:"sensitive"`
}

type StateFileResource struct {
	Module    string              `json:"module,omitempty"`
	Mode      string              `json:"mode"`
	Type      string              `json:"type"`
	Name      string              `json:"name"`
	Each      string              `json:"each,omitempty"`
	Provider  string              `json:"provider"`
	Instances []StateFileInstance `json:"instances"`
}

func (r StateFileResource) FullyQualifiedName() string {
	fullName := fmt.Sprintf("%s.%s", r.Type, r.Name)
	if r.Module == "data" {
		fullName = "data." + fullName
	}
	if r.Module != "" {
		fullName = r.Module + "." + fullName
	}
	return fullName
}

type StateFileInstance struct {
	IndexKey interface{} `json:"index_key,omitempty"`
	Status   string      `json:"status,omitempty"`
	Deposed  string      `json:"deposed,omitempty"`

	SchemaVersion           uint64            `json:"schema_version"`
	AttributesRaw           json.RawMessage   `json:"attributes,omitempty"`
	AttributesFlat          map[string]string `json:"attributes_flat,omitempty"`
	AttributeSensitivePaths json.RawMessage   `json:"sensitive_attributes,omitempty"`

	IdentitySchemaVersion uint64          `json:"identity_schema_version"`
	IdentityRaw           json.RawMessage `json:"identity,omitempty"`

	PrivateRaw []byte `json:"private,omitempty"`

	Dependencies []string `json:"dependencies,omitempty"`

	CreateBeforeDestroy bool `json:"create_before_destroy,omitempty"`
}

type StateFileCheckResult struct {
	ObjectKind string                       `json:"object_kind"`
	ConfigAddr string                       `json:"config_addr"`
	Status     string                       `json:"status"`
	Objects    []StateFileCheckResultObject `json:"objects"`
}

type StateFileCheckResultObject struct {
	ObjectAddr      string   `json:"object_addr"`
	Status          string   `json:"status"`
	FailureMessages []string `json:"failure_messages,omitempty"`
}
