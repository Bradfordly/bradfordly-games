package panel

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type fakeSSM struct {
	out *ssm.GetParameterOutput
	err error
}

func (f fakeSSM) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return f.out, f.err
}

func TestSSMStoreGet(t *testing.T) {
	ctx := context.Background()
	if _, err := (SSMStore{}).Get(ctx, "/x"); err == nil {
		t.Fatal("nil client must error")
	}

	down := NewSSMStore(fakeSSM{err: errors.New("denied")})
	if _, err := down.Get(ctx, "/x"); err == nil {
		t.Fatal("api error must surface")
	}

	empty := NewSSMStore(fakeSSM{out: &ssm.GetParameterOutput{Parameter: &types.Parameter{}}})
	if _, err := empty.Get(ctx, "/x"); err == nil {
		t.Fatal("empty value must error")
	}

	ok := NewSSMStore(fakeSSM{out: &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("owner@example.com")}}})
	got, err := ok.Get(ctx, "/allow")
	if err != nil || got != "owner@example.com" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
