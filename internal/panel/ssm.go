package panel

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// ParameterStore reads a named parameter. Used for the allowlist and
// the GitHub client secret.
type ParameterStore interface {
	Get(ctx context.Context, name string) (string, error)
}

type ssmAPI interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// SSMStore reads SecureString and String parameters from AWS SSM.
type SSMStore struct {
	api ssmAPI
}

// NewSSMStore wraps an SSM client.
func NewSSMStore(api ssmAPI) SSMStore {
	return SSMStore{api: api}
}

// Get returns the parameter value. Missing or empty values are an error.
func (s SSMStore) Get(ctx context.Context, name string) (string, error) {
	if s.api == nil {
		return "", errors.New("ssm client is not configured")
	}
	out, err := s.api.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", err
	}
	if out == nil || out.Parameter == nil || out.Parameter.Value == nil || *out.Parameter.Value == "" {
		return "", errors.New("ssm parameter has no value")
	}
	return *out.Parameter.Value, nil
}
