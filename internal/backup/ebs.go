package backup

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// EC2 is the CreateSnapshot surface used for on-demand backups.
type EC2 interface {
	CreateSnapshot(ctx context.Context, params *ec2.CreateSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.CreateSnapshotOutput, error)
}

// EBS creates on-demand snapshots of the data volume.
type EBS struct {
	EC2 EC2
}

// Create issues CreateSnapshot against the #57 data volume.
func (e *EBS) Create(ctx context.Context, req Request) (Snapshot, error) {
	if e == nil || e.EC2 == nil {
		return Snapshot{}, ErrSnapshotFailed
	}
	if strings.TrimSpace(req.VolumeID) == "" {
		return Snapshot{}, ErrMissingVolume
	}

	input := &ec2.CreateSnapshotInput{
		VolumeId:    aws.String(req.VolumeID),
		Description: aws.String(req.Description),
	}
	if len(req.Tags) > 0 {
		tags := make([]types.Tag, 0, len(req.Tags))
		for key, value := range req.Tags {
			tags = append(tags, types.Tag{
				Key:   aws.String(key),
				Value: aws.String(value),
			})
		}
		input.TagSpecifications = []types.TagSpecification{{
			ResourceType: types.ResourceTypeSnapshot,
			Tags:         tags,
		}}
	}

	out, err := e.EC2.CreateSnapshot(ctx, input)
	if err != nil {
		return Snapshot{}, err
	}
	if out == nil || out.SnapshotId == nil || *out.SnapshotId == "" {
		return Snapshot{}, ErrSnapshotFailed
	}
	return Snapshot{
		ID:          *out.SnapshotId,
		VolumeID:    req.VolumeID,
		Description: req.Description,
	}, nil
}
