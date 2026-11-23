package platform

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// IPAddress is an address of an instance's network interface.
type IPAddress struct {
	Address string `json:"ipAddress"`
	Type    string `json:"ipAddressType"`
	Prefix  int    `json:"prefix"`
}

// NetworkInterface is a network interface as the running instance reports it.
type NetworkInterface struct {
	Name        string      `json:"name"`
	IPAddresses []IPAddress `json:"ipAddresses"`
}

// Instance is a virtual machine of the tenant.
type Instance struct {
	ID     int     `json:"vmid"`
	Name   *string `json:"name"`
	Status *string `json:"status"`
	CPUs   *int    `json:"cpus"`
	// MemoryBytes and DiskBytes are the sizes the instance was given.
	MemoryBytes   *float64 `json:"maxmem"`
	DiskBytes     *float64 `json:"maxdisk"`
	UptimeSeconds *int     `json:"uptime"`
	Config        struct {
		Cores       *int    `json:"cores"`
		Memory      *string `json:"memory"`
		Description *string `json:"description"`
	} `json:"config"`
	Interfaces *struct {
		Items []NetworkInterface `json:"items"`
	} `json:"networkInterfaces"`
}

// Running reports whether the instance is powered on.
func (i *Instance) Running() bool { return i != nil && i.Status != nil && *i.Status == "running" }

const instanceFields = `vmid name status cpus maxmem maxdisk uptime config { cores memory description }`

// ListInstances lists the tenant's instances.
func (c *Client) ListInstances(ctx context.Context) ([]Instance, error) {
	var out struct {
		Instances struct {
			Items []Instance `json:"items"`
		} `json:"instances"`
	}
	err := c.Do(ctx, `query { instances { items { `+instanceFields+` } } }`, nil, &out)
	return out.Instances.Items, err
}

// GetInstance returns one instance with its network interfaces.
func (c *Client) GetInstance(ctx context.Context, id int) (*Instance, error) {
	var out struct {
		Instance Instance `json:"instance"`
	}
	err := c.Do(ctx, `query($id: ID!) { instance(id: $id) { `+instanceFields+` networkInterfaces { items { name ipAddresses { ipAddress ipAddressType prefix } } } } }`,
		map[string]any{"id": strconv.Itoa(id)}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Instance, nil
}

// FindInstanceByName returns the instance with the given name, or nil.
func (c *Client) FindInstanceByName(ctx context.Context, name string) (*Instance, error) {
	instances, err := c.ListInstances(ctx)
	if err != nil {
		return nil, err
	}
	for i := range instances {
		if instances[i].Name != nil && *instances[i].Name == name {
			return &instances[i], nil
		}
	}
	return nil, nil
}

// InstanceTemplate is an operating system image instances are created from.
type InstanceTemplate struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// ListInstanceTemplates lists the images instances can be created from.
func (c *Client) ListInstanceTemplates(ctx context.Context) ([]InstanceTemplate, error) {
	var out struct {
		Templates struct {
			Items []InstanceTemplate `json:"items"`
		} `json:"instanceTemplates"`
	}
	err := c.Do(ctx, `query { instanceTemplates { items { id name description } } }`, nil, &out)
	return out.Templates.Items, err
}

// Disk types of an additional volume.
const (
	DiskSSD = "SSD"
	DiskHDD = "HDD"
)

// InstanceVolume is an additional disk for a new instance.
type InstanceVolume struct {
	SizeGB int    `json:"size"`
	Type   string `json:"type"`
}

// CreateInstanceInput describes a new instance.
type CreateInstanceInput struct {
	Name string `json:"name"`
	// Template is the id of an InstanceTemplate.
	Template string `json:"templateVmId"`
	CPUs     int    `json:"cpus"`
	// MemoryMB is the memory in mebibytes.
	MemoryMB    int     `json:"memory"`
	Description *string `json:"description,omitempty"`
	// RootDiskGB grows the image's root disk; zero keeps the image's size.
	RootDiskGB int `json:"rootDiskSize,omitempty"`
	// SSHKeyIDs are ids of keys in the tenant's key store.
	SSHKeyIDs []string `json:"sshKeyIds,omitempty"`
	// SSHKey is a public key given inline.
	SSHKey            *string          `json:"sshKey,omitempty"`
	Password          *string          `json:"password,omitempty"`
	AdditionalVolumes []InstanceVolume `json:"additionalVolumes,omitempty"`
}

// CreateInstance starts creating an instance and returns the id of the
// background task that does it. CreateInstanceAndWait waits for the result.
func (c *Client) CreateInstance(ctx context.Context, in CreateInstanceInput) (string, error) {
	var out struct {
		TaskID string `json:"createInstance"`
	}
	err := c.Do(ctx, `mutation($input: CreateInstanceInput!) { createInstance(input: $input) }`, map[string]any{"input": in}, &out)
	return out.TaskID, err
}

// CreateInstanceAndWait creates an instance, waits for the background task
// and returns the instance.
func (c *Client) CreateInstanceAndWait(ctx context.Context, in CreateInstanceInput) (*Instance, error) {
	task, err := c.CreateInstance(ctx, in)
	if err != nil {
		return nil, err
	}
	if _, err := c.WaitForTask(ctx, task, 2*time.Second); err != nil {
		return nil, err
	}
	instance, err := c.FindInstanceByName(ctx, in.Name)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, errors.New("platform: the instance was created but is not listed yet")
	}
	return instance, nil
}

// StartInstance powers an instance on.
func (c *Client) StartInstance(ctx context.Context, id int) error {
	return c.Do(ctx, `mutation($id: Int!) { startInstance(vmid: $id) }`, map[string]any{"id": id}, nil)
}

// StopInstance powers an instance off.
func (c *Client) StopInstance(ctx context.Context, id int) error {
	return c.Do(ctx, `mutation($id: Int!) { stopInstance(vmid: $id) }`, map[string]any{"id": id}, nil)
}

// RebootInstance restarts an instance.
func (c *Client) RebootInstance(ctx context.Context, id int) error {
	return c.Do(ctx, `mutation($id: Int!) { rebootInstance(vmid: $id) }`, map[string]any{"id": id}, nil)
}

// DeleteInstance deletes an instance and its disks.
func (c *Client) DeleteInstance(ctx context.Context, id int) error {
	return c.Do(ctx, `mutation($id: Int!) { deleteInstance(vmid: $id) }`, map[string]any{"id": id}, nil)
}

// ResizeInstance changes the CPUs and the memory (in mebibytes) of an
// instance; zero leaves a value as it is.
func (c *Client) ResizeInstance(ctx context.Context, id, cpus, memoryMB int) error {
	input := map[string]any{"vmid": id}
	if cpus > 0 {
		input["cpus"] = cpus
	}
	if memoryMB > 0 {
		input["memory"] = memoryMB
	}
	return c.Do(ctx, `mutation($input: ResizeInstanceInput!) { resizeInstance(input: $input) }`, map[string]any{"input": input}, nil)
}

// ResizeInstanceDisk grows a disk of an instance to sizeGB. disk names the
// disk as the instance's volumes list it.
func (c *Client) ResizeInstanceDisk(ctx context.Context, id int, disk string, sizeGB int) error {
	return c.Do(ctx, `mutation($input: ResizeInstanceDiskInput!) { resizeInstanceDisk(input: $input) }`,
		map[string]any{"input": map[string]any{"vmid": id, "disk": disk, "size": sizeGB}}, nil)
}

// WaitInstanceStatus polls until the instance reports the wanted status,
// for example "running" or "stopped".
func (c *Client) WaitInstanceStatus(ctx context.Context, id int, status string, o WaitOptions) (*Instance, error) {
	var instance *Instance
	err := waitReady(ctx, "instance "+strconv.Itoa(id), o, func(ctx context.Context) (Status, string, error) {
		var err error
		if instance, err = c.GetInstance(ctx, id); err != nil {
			return "", "", err
		}
		if instance.Status != nil && *instance.Status == status {
			return StatusReady, "", nil
		}
		return StatusPending, "", nil
	})
	return instance, err
}
