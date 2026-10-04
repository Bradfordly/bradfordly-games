package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type fakeEC2 struct {
	out   *ec2.CreateSnapshotOutput
	err   error
	input *ec2.CreateSnapshotInput
}

func (f *fakeEC2) CreateSnapshot(_ context.Context, params *ec2.CreateSnapshotInput, _ ...func(*ec2.Options)) (*ec2.CreateSnapshotOutput, error) {
	f.input = params
	return f.out, f.err
}

func TestEBSCreateSnapshot(t *testing.T) {
	api := &fakeEC2{out: &ec2.CreateSnapshotOutput{SnapshotId: aws.String("snap-9")}}
	ebs := &EBS{EC2: api}

	got, err := ebs.Create(context.Background(), Request{
		VolumeID:    "vol-57",
		Description: "pre-delete world=survival",
		Tags:        map[string]string{"Purpose": "pre-delete", "WorldID": "survival"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "snap-9" || got.VolumeID != "vol-57" {
		t.Fatalf("got %#v", got)
	}
	if aws.ToString(api.input.VolumeId) != "vol-57" {
		t.Fatalf("volume id = %v", api.input.VolumeId)
	}
	if aws.ToString(api.input.Description) != "pre-delete world=survival" {
		t.Fatalf("description = %v", api.input.Description)
	}
	if len(api.input.TagSpecifications) != 1 {
		t.Fatal("expected snapshot tags")
	}
	spec := api.input.TagSpecifications[0]
	if spec.ResourceType != types.ResourceTypeSnapshot {
		t.Fatalf("resource type = %s", spec.ResourceType)
	}
	if len(spec.Tags) != 2 {
		t.Fatalf("tags = %#v", spec.Tags)
	}
}

func TestEBSCreateSnapshotErrors(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var ebs *EBS
		if _, err := ebs.Create(context.Background(), Request{VolumeID: "vol-1"}); !errors.Is(err, ErrSnapshotFailed) {
			t.Fatalf("err = %v", err)
		}
		ebs = &EBS{}
		if _, err := ebs.Create(context.Background(), Request{VolumeID: "vol-1"}); !errors.Is(err, ErrSnapshotFailed) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("missing volume", func(t *testing.T) {
		ebs := &EBS{EC2: &fakeEC2{}}
		if _, err := ebs.Create(context.Background(), Request{}); !errors.Is(err, ErrMissingVolume) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		ebs := &EBS{EC2: &fakeEC2{err: errors.New("denied")}}
		if _, err := ebs.Create(context.Background(), Request{VolumeID: "vol-1"}); err == nil {
			t.Fatal("expected api error")
		}
	})
	t.Run("empty snapshot id", func(t *testing.T) {
		ebs := &EBS{EC2: &fakeEC2{out: &ec2.CreateSnapshotOutput{}}}
		if _, err := ebs.Create(context.Background(), Request{VolumeID: "vol-1"}); !errors.Is(err, ErrSnapshotFailed) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("nil output", func(t *testing.T) {
		ebs := &EBS{EC2: &fakeEC2{}}
		if _, err := ebs.Create(context.Background(), Request{VolumeID: "vol-1"}); !errors.Is(err, ErrSnapshotFailed) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no tags", func(t *testing.T) {
		api := &fakeEC2{out: &ec2.CreateSnapshotOutput{SnapshotId: aws.String("snap-2")}}
		ebs := &EBS{EC2: api}
		if _, err := ebs.Create(context.Background(), Request{VolumeID: "vol-1"}); err != nil {
			t.Fatal(err)
		}
		if len(api.input.TagSpecifications) != 0 {
			t.Fatalf("tags = %#v", api.input.TagSpecifications)
		}
	})
}
