// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/vm"
)

var _ = Describe("BuildVMSpec", func() {
	var (
		opts   vm.VMSpecOpts
		result *kubevirtv1.VirtualMachine
	)

	BeforeEach(func() {
		opts = vm.VMSpecOpts{
			Name:                "test-vm",
			Namespace:           "test-ns",
			ContainerDiskImage:  "quay.io/containerdisks/fedora:41",
			CloudInitSecretName: "test-vm-cloudinit",
			CPUCores:            2,
			Memory:              "2Gi",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "virtwork",
				"app.kubernetes.io/name":       "cpu",
			},
		}
		var err error
		result, err = vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should set correct API version and kind", func() {
		Expect(result.APIVersion).To(Equal("kubevirt.io/v1"))
		Expect(result.Kind).To(Equal("VirtualMachine"))
	})

	It("should set name and namespace", func() {
		Expect(result.Name).To(Equal("test-vm"))
		Expect(result.Namespace).To(Equal("test-ns"))
	})

	It("should configure containerDisk volume", func() {
		volumes := result.Spec.Template.Spec.Volumes
		var containerDisk *kubevirtv1.Volume
		for i := range volumes {
			if volumes[i].Name == "containerdisk" {
				containerDisk = &volumes[i]
				break
			}
		}
		Expect(containerDisk).NotTo(BeNil())
		Expect(containerDisk.ContainerDisk).NotTo(BeNil())
		Expect(containerDisk.ContainerDisk.Image).To(Equal("quay.io/containerdisks/fedora:41"))
	})

	It("should configure cloudInitNoCloud volume", func() {
		volumes := result.Spec.Template.Spec.Volumes
		var cloudInit *kubevirtv1.Volume
		for i := range volumes {
			if volumes[i].Name == "cloudinitdisk" {
				cloudInit = &volumes[i]
				break
			}
		}
		Expect(cloudInit).NotTo(BeNil())
		Expect(cloudInit.CloudInitNoCloud).NotTo(BeNil())
		Expect(cloudInit.CloudInitNoCloud.UserDataSecretRef).NotTo(BeNil())
		Expect(cloudInit.CloudInitNoCloud.UserDataSecretRef.Name).To(Equal("test-vm-cloudinit"))
	})

	It("should set labels", func() {
		Expect(result.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "virtwork"))
		Expect(result.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "cpu"))
		// Labels should also propagate to the template
		Expect(
			result.Spec.Template.ObjectMeta.Labels,
		).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "virtwork"))
	})

	It("should set CPU and memory resources", func() {
		domain := result.Spec.Template.Spec.Domain
		Expect(domain.CPU).NotTo(BeNil())
		Expect(domain.CPU.Cores).To(Equal(uint32(2)))
		memReq := domain.Resources.Requests[corev1.ResourceMemory]
		expected := resource.MustParse("2Gi")
		Expect(memReq.Equal(expected)).To(BeTrue())
	})

	It("should set RunStrategy to Always", func() {
		Expect(result.Spec.RunStrategy).NotTo(BeNil())
		Expect(*result.Spec.RunStrategy).To(Equal(kubevirtv1.RunStrategyAlways))
	})

	It("should configure masquerade networking", func() {
		interfaces := result.Spec.Template.Spec.Domain.Devices.Interfaces
		Expect(interfaces).To(HaveLen(1))
		Expect(interfaces[0].Name).To(Equal("default"))
		Expect(interfaces[0].Masquerade).NotTo(BeNil())

		networks := result.Spec.Template.Spec.Networks
		Expect(networks).To(HaveLen(1))
		Expect(networks[0].Name).To(Equal("default"))
		Expect(networks[0].Pod).NotTo(BeNil())
	})

	It("should use UserDataSecretRef from CloudInitSecretName", func() {
		var err error
		result, err = vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())

		volumes := result.Spec.Template.Spec.Volumes
		var cloudInit *kubevirtv1.Volume
		for i := range volumes {
			if volumes[i].Name == "cloudinitdisk" {
				cloudInit = &volumes[i]
				break
			}
		}
		Expect(cloudInit).NotTo(BeNil())
		Expect(cloudInit.CloudInitNoCloud).NotTo(BeNil())
		Expect(cloudInit.CloudInitNoCloud.UserDataSecretRef).NotTo(BeNil())
		Expect(cloudInit.CloudInitNoCloud.UserDataSecretRef.Name).To(Equal("test-vm-cloudinit"))
		Expect(cloudInit.CloudInitNoCloud.UserData).To(BeEmpty())
	})

	It("should return error when CloudInitSecretName is empty", func() {
		opts.CloudInitSecretName = ""
		_, err := vm.BuildVMSpec(opts)
		Expect(err).To(MatchError(vm.ErrInvalidCloudInitSecretName))
	})

	It("should include extra disks when provided", func() {
		opts.ExtraDisks = []kubevirtv1.Disk{
			{
				Name: "datadisk",
				DiskDevice: kubevirtv1.DiskDevice{
					Disk: &kubevirtv1.DiskTarget{
						Bus: "virtio",
					},
				},
			},
		}
		opts.ExtraVolumes = []kubevirtv1.Volume{
			{
				Name: "datadisk",
				VolumeSource: kubevirtv1.VolumeSource{
					DataVolume: &kubevirtv1.DataVolumeSource{
						Name: "test-data",
					},
				},
			},
		}
		var err error
		result, err = vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())

		disks := result.Spec.Template.Spec.Domain.Devices.Disks
		Expect(disks).To(HaveLen(3)) // containerdisk + cloudinitdisk + datadisk

		volumes := result.Spec.Template.Spec.Volumes
		Expect(volumes).To(HaveLen(3))
	})

	It("should include data volume templates when provided", func() {
		dvt, err := vm.BuildDataVolumeTemplate("test-data", "10Gi")
		Expect(err).NotTo(HaveOccurred())
		opts.DataVolumeTemplates = []kubevirtv1.DataVolumeTemplateSpec{dvt}
		result, err = vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())

		Expect(result.Spec.DataVolumeTemplates).To(HaveLen(1))
		Expect(result.Spec.DataVolumeTemplates[0].Name).To(Equal("test-data"))
	})

	Context("input validation", func() {
		DescribeTable(
			"should reject invalid inputs",
			func(mutate func(*vm.VMSpecOpts), substr string) {
				good := vm.VMSpecOpts{
					Name:                "test-vm",
					Namespace:           "test-ns",
					ContainerDiskImage:  "quay.io/containerdisks/fedora:41",
					CloudInitSecretName: "test-cloudinit",
					CPUCores:            2,
					Memory:              "2Gi",
					Labels:              map[string]string{"test": "true"},
				}
				mutate(&good)
				_, err := vm.BuildVMSpec(good)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(substr))
			},
			Entry("empty name", func(o *vm.VMSpecOpts) { o.Name = "" }, "name"),
			Entry("empty namespace", func(o *vm.VMSpecOpts) { o.Namespace = "" }, "namespace"),
			Entry("zero CPU cores", func(o *vm.VMSpecOpts) { o.CPUCores = 0 }, "cpuCores"),
			Entry("negative CPU cores", func(o *vm.VMSpecOpts) { o.CPUCores = -1 }, "cpuCores"),
			Entry(
				"empty container disk image",
				func(o *vm.VMSpecOpts) { o.ContainerDiskImage = "" },
				"containerDiskImage",
			),
			Entry(
				"empty cloud init secret name",
				func(o *vm.VMSpecOpts) { o.CloudInitSecretName = "" },
				"cloudInitSecretName",
			),
		)
	})
})

var _ = Describe("BuildDataVolumeTemplate", func() {
	It("should set name", func() {
		dvt, err := vm.BuildDataVolumeTemplate("data-disk", "20Gi")
		Expect(err).NotTo(HaveOccurred())
		Expect(dvt.Name).To(Equal("data-disk"))
	})

	It("should set blank source", func() {
		dvt, err := vm.BuildDataVolumeTemplate("data-disk", "20Gi")
		Expect(err).NotTo(HaveOccurred())
		Expect(dvt.Spec.Source).NotTo(BeNil())
		Expect(dvt.Spec.Source.Blank).NotTo(BeNil())
	})

	It("should set storage size", func() {
		dvt, err := vm.BuildDataVolumeTemplate("data-disk", "20Gi")
		Expect(err).NotTo(HaveOccurred())
		Expect(dvt.Spec.Storage).NotTo(BeNil())
		storageReq := dvt.Spec.Storage.Resources.Requests[corev1.ResourceStorage]
		expected := resource.MustParse("20Gi")
		Expect(storageReq.Equal(expected)).To(BeTrue())
	})

	It("should return error for invalid disk size", func() {
		_, err := vm.BuildDataVolumeTemplate("data-disk", "not-a-size")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid disk size"))
	})
})

var _ = Describe("CreateVM", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
		restore := vm.SetBaseRetryBackoff(time.Millisecond)
		DeferCleanup(restore)
	})

	newTestVM := func(name string) *kubevirtv1.VirtualMachine {
		v, err := vm.BuildVMSpec(vm.VMSpecOpts{
			Name:                name,
			Namespace:           "default",
			ContainerDiskImage:  "test-image",
			CloudInitSecretName: "test-cloudinit",
			CPUCores:            1,
			Memory:              "1Gi",
			Labels:              map[string]string{"test": "true"},
		})
		Expect(err).NotTo(HaveOccurred())
		return v
	}

	It("should return error for invalid memory", func() {
		_, err := vm.BuildVMSpec(vm.VMSpecOpts{
			Name:                "bad-mem",
			Namespace:           "default",
			ContainerDiskImage:  "test-image",
			CloudInitSecretName: "test-cloudinit",
			CPUCores:            1,
			Memory:              "not-a-quantity",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid memory"))
	})

	It("should create VM successfully", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		testVM := newTestVM("test-vm")

		err := vm.CreateVM(ctx, c, testVM)
		Expect(err).NotTo(HaveOccurred())

		got := &kubevirtv1.VirtualMachine{}
		err = c.Get(ctx, client.ObjectKeyFromObject(testVM), got)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Name).To(Equal("test-vm"))
	})

	It("should skip on AlreadyExists", func() {
		testVM := newTestVM("existing-vm")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testVM).Build()

		// Creating the same VM again should not error
		dupVM := newTestVM("existing-vm")
		err := vm.CreateVM(ctx, c, dupVM)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should retry on transient errors", func() {
		callCount := 0
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					callCount++
					if callCount <= 2 {
						return apierrors.NewServiceUnavailable("temporarily unavailable")
					}
					return cl.Create(ctx, obj, opts...)
				},
			}).
			Build()

		testVM := newTestVM("retry-vm")
		err := vm.CreateVM(ctx, c, testVM)
		Expect(err).NotTo(HaveOccurred())
		Expect(callCount).To(BeNumerically(">=", 3))
	})

	It("should fail on NotFound", func() {
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					return apierrors.NewNotFound(
						schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachines"},
						"test-vm",
					)
				},
			}).
			Build()

		testVM := newTestVM("test-vm")
		err := vm.CreateVM(ctx, c, testVM)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("should fail on Unauthorized", func() {
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					return apierrors.NewUnauthorized("unauthorized")
				},
			}).
			Build()

		testVM := newTestVM("test-vm")
		err := vm.CreateVM(ctx, c, testVM)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsUnauthorized(err)).To(BeTrue())
	})
})

var _ = Describe("DeleteVM", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
		restore := vm.SetBaseRetryBackoff(time.Millisecond)
		DeferCleanup(restore)
	})

	It("should delete VM successfully", func() {
		testVM, err := vm.BuildVMSpec(vm.VMSpecOpts{
			Name:                "delete-me",
			Namespace:           "default",
			ContainerDiskImage:  "test-image",
			CloudInitSecretName: "test-cloudinit",
			CPUCores:            1,
			Memory:              "1Gi",
		})
		Expect(err).NotTo(HaveOccurred())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testVM).Build()

		err = vm.DeleteVM(ctx, c, "delete-me", "default")
		Expect(err).NotTo(HaveOccurred())

		got := &kubevirtv1.VirtualMachine{}
		err = c.Get(ctx, client.ObjectKey{Name: "delete-me", Namespace: "default"}, got)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("should skip on NotFound", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		err := vm.DeleteVM(ctx, c, "nonexistent", "default")
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("ListVMs", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should list VMs by labels", func() {
		vm1, err := vm.BuildVMSpec(vm.VMSpecOpts{
			Name:                "vm-1",
			Namespace:           "default",
			ContainerDiskImage:  "test-image",
			CloudInitSecretName: "test-cloudinit",
			CPUCores:            1,
			Memory:              "1Gi",
			Labels:              map[string]string{"app.kubernetes.io/managed-by": "virtwork"},
		})
		Expect(err).NotTo(HaveOccurred())
		vm2, err := vm.BuildVMSpec(vm.VMSpecOpts{
			Name:                "vm-2",
			Namespace:           "default",
			ContainerDiskImage:  "test-image",
			CloudInitSecretName: "test-cloudinit",
			CPUCores:            1,
			Memory:              "1Gi",
			Labels:              map[string]string{"app.kubernetes.io/managed-by": "other"},
		})
		Expect(err).NotTo(HaveOccurred())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm1, vm2).Build()

		vms, err := vm.ListVMs(ctx, c, "default", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(HaveLen(1))
		Expect(vms[0].Name).To(Equal("vm-1"))
	})

	It("should return empty list when no VMs match", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		vms, err := vm.ListVMs(ctx, c, "default", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(BeEmpty())
	})
})

var _ = Describe("GetVMIPhase", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should return VMI phase", func() {
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vmi",
				Namespace: "default",
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Running,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmi).Build()

		phase, err := vm.GetVMIPhase(ctx, c, "test-vmi", "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).To(Equal(kubevirtv1.Running))
	})

	It("should return error for nonexistent VMI", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		_, err := vm.GetVMIPhase(ctx, c, "nonexistent", "default")
		Expect(err).To(HaveOccurred())
	})
})
