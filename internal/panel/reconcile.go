package panel

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/bradfordly/bradfordly-games/internal/world"
)

// Example EFS identifiers from docs/ADRs/ADR-0004 and docs/specs/operations.md.
// They are the static-PV convention, not a live AWS account.
const (
	exampleFileSystemID = "fs-example"
	efsCSIDriver        = "efs.csi.aws.com"
	efsStorageClass     = "efs-static"
	minecraftMountPath  = "/data"
	defaultWorldNS      = "worlds"
)

// Reconciler ensures Kubernetes objects for a world spec.
type Reconciler interface {
	EnsureSleeping(ctx context.Context, w world.World) error
}

// Cluster applies core objects. Tests use MemoryCluster.
type Cluster interface {
	ApplyPV(ctx context.Context, pv *corev1.PersistentVolume) error
	ApplyPVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error
	ApplyService(ctx context.Context, svc *corev1.Service) error
	ApplyStatefulSet(ctx context.Context, sts *appsv1.StatefulSet) error
}

// MemoryCluster holds applied objects in process.
type MemoryCluster struct {
	PVs          map[string]*corev1.PersistentVolume
	PVCs         map[string]*corev1.PersistentVolumeClaim
	Services     map[string]*corev1.Service
	StatefulSets map[string]*appsv1.StatefulSet
}

// NewMemoryCluster returns an empty cluster double.
func NewMemoryCluster() *MemoryCluster {
	return &MemoryCluster{
		PVs:          map[string]*corev1.PersistentVolume{},
		PVCs:         map[string]*corev1.PersistentVolumeClaim{},
		Services:     map[string]*corev1.Service{},
		StatefulSets: map[string]*appsv1.StatefulSet{},
	}
}

func (c *MemoryCluster) ApplyPV(_ context.Context, pv *corev1.PersistentVolume) error {
	c.PVs[pv.Name] = pv
	return nil
}

func (c *MemoryCluster) ApplyPVC(_ context.Context, pvc *corev1.PersistentVolumeClaim) error {
	c.PVCs[pvc.Name] = pvc
	return nil
}

func (c *MemoryCluster) ApplyService(_ context.Context, svc *corev1.Service) error {
	c.Services[svc.Name] = svc
	return nil
}

func (c *MemoryCluster) ApplyStatefulSet(_ context.Context, sts *appsv1.StatefulSet) error {
	c.StatefulSets[sts.Name] = sts
	return nil
}

// SleepingReconciler creates the PVC, ClusterIP, and replicas=0 StatefulSet.
type SleepingReconciler struct {
	Cluster      Cluster
	Namespace    string
	FileSystemID string
}

// NewSleepingReconciler builds a Minecraft sleeping reconciler.
func NewSleepingReconciler(cluster Cluster, namespace, fileSystemID string) *SleepingReconciler {
	if namespace == "" {
		namespace = defaultWorldNS
	}
	if fileSystemID == "" {
		fileSystemID = exampleFileSystemID
	}
	return &SleepingReconciler{Cluster: cluster, Namespace: namespace, FileSystemID: fileSystemID}
}

func sleepingReconcilerFromEnv() *SleepingReconciler {
	ns := os.Getenv("PANEL_WORLD_NAMESPACE")
	fs := os.Getenv("PANEL_EFS_FILE_SYSTEM_ID")
	return NewSleepingReconciler(NewMemoryCluster(), ns, fs)
}

// EnsureSleeping writes the EFS static volume, ClusterIP Service, and a
// sleeping Minecraft StatefulSet. It does not start the pod.
func (r *SleepingReconciler) EnsureSleeping(ctx context.Context, w world.World) error {
	if r == nil || r.Cluster == nil {
		return nil
	}
	rec := w.Record()
	world.ApplyDefaults(&rec)
	if rec.Game != world.GameMinecraftJava {
		return fmt.Errorf("reconcile: unsupported game %q", rec.Game)
	}

	pv, pvc := efsVolume(r.Namespace, rec, r.FileSystemID)
	if err := r.Cluster.ApplyPV(ctx, pv); err != nil {
		return err
	}
	if err := r.Cluster.ApplyPVC(ctx, pvc); err != nil {
		return err
	}
	if err := r.Cluster.ApplyService(ctx, minecraftService(r.Namespace, rec)); err != nil {
		return err
	}
	return r.Cluster.ApplyStatefulSet(ctx, sleepingMinecraftSTS(r.Namespace, rec, pvc.Name))
}

func efsVolume(namespace string, rec world.Record, fileSystemID string) (*corev1.PersistentVolume, *corev1.PersistentVolumeClaim) {
	accessPoint := "fsap-" + rec.ID
	pvName := rec.Volume + "-pv"
	storage := resource.MustParse("20Gi")
	mode := corev1.PersistentVolumeFilesystem
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: pvName},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: storage},
			VolumeMode:                    &mode,
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              efsStorageClass,
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       efsCSIDriver,
					VolumeHandle: fileSystemID + "::" + accessPoint,
				},
			},
		},
	}
	sc := efsStorageClass
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: rec.Volume, Namespace: namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			StorageClassName: &sc,
			VolumeName:       pvName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: storage},
			},
		},
	}
	return pv, pvc
}

func minecraftService(namespace string, rec world.Record) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: rec.Backend.Service, Namespace: namespace},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"world": rec.ID,
			},
			Ports: []corev1.ServicePort{{
				Name:       "minecraft",
				Port:       world.DefaultMCPort,
				TargetPort: intstr.FromInt(world.DefaultMCPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

func sleepingMinecraftSTS(namespace string, rec world.Record, pvcName string) *appsv1.StatefulSet {
	replicas := int32(0)
	grace := int64(DefaultStopGrace(rec.StopTimeout).Seconds())
	env := []corev1.EnvVar{{Name: "EULA", Value: "TRUE"}}
	for k, v := range rec.Env {
		if k == "EULA" {
			env[0].Value = v
			continue
		}
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: rec.Backend.Workload, Namespace: namespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: rec.Backend.Service,
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"world": rec.ID}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"world": rec.ID}},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &grace,
					Containers: []corev1.Container{{
						Name:  "minecraft",
						Image: rec.Image,
						Env:   env,
						Ports: []corev1.ContainerPort{{
							Name:          "minecraft",
							ContainerPort: world.DefaultMCPort,
							Protocol:      corev1.ProtocolTCP,
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "save",
							MountPath: minecraftMountPath,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "save",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
						},
					}},
				},
			},
		},
	}
}

// DefaultStopGrace is terminationGracePeriodSeconds from stop_timeout (min 2m).
func DefaultStopGrace(stopTimeout string) time.Duration {
	d, err := time.ParseDuration(stopTimeout)
	if err != nil || d < world.DefaultStopTimeout {
		return world.DefaultStopTimeout
	}
	return d
}
