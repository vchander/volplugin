package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// VolumeConfig is the configuration of the tenant. It includes pool and
// snapshot information.
type VolumeConfig struct {
	Name       string        `json:"name"`
	Pool       string        `json:"pool"`
	VolumeName string        `json:"name"`
	Options    VolumeOptions `json:"options"`
}

// VolumeOptions comprises the optional paramters a volume can accept.
type VolumeOptions struct {
	Size         uint64         `json:"size" merge:"size"`
	UseSnapshots bool           `json:"snapshots" merge:"snapshots"`
	Snapshot     SnapshotConfig `json:"snapshot"`
}

// SnapshotConfig is the configuration for snapshots.
type SnapshotConfig struct {
	Frequency string `json:"frequency" merge:"snapshots.frequency"`
	Keep      uint   `json:"keep" merge:"snapshots.keep"`
}

func (c *TopLevelConfig) volume(pool, name string) string {
	return c.prefixed(rootVolume, pool, name)
}

// CreateVolume sets the appropriate config metadata for a volume creation
// operation, and returns the VolumeConfig that was copied in.
func (c *TopLevelConfig) CreateVolume(name, tenant, pool string, opts map[string]string) (*VolumeConfig, error) {
	if tc, err := c.GetVolume(name, pool); err == nil {
		return tc, ErrExist
	}

	resp, err := c.GetTenant(tenant)
	if err != nil {
		return nil, err
	}

	if err := mergeOpts(&resp.DefaultVolumeOptions, opts); err != nil {
		return nil, err
	}

	vc := VolumeConfig{
		Name:       name,
		Options:    resp.DefaultVolumeOptions,
		VolumeName: name,
		Pool:       pool,
	}

	remarshal, err := json.Marshal(vc)
	if err != nil {
		return nil, err
	}

	if _, err := c.etcdClient.Set(c.volume(pool, name), string(remarshal), 0); err != nil {
		return nil, err
	}

	return &vc, nil
}

// GetVolume returns the VolumeConfig for a given volume.
func (c *TopLevelConfig) GetVolume(pool, name string) (*VolumeConfig, error) {
	resp, err := c.etcdClient.Get(c.volume(pool, name), true, false)
	if err != nil {
		return nil, err
	}

	ret := &VolumeConfig{}

	if err := json.Unmarshal([]byte(resp.Node.Value), ret); err != nil {
		return nil, err
	}

	return ret, nil
}

// RemoveVolume removes a volume from configuration.
func (c *TopLevelConfig) RemoveVolume(pool, name string) error {
	_, err := c.etcdClient.Delete(c.prefixed(rootVolume, pool, name), true)
	return err
}

// ListVolumes returns a map of volume name -> VolumeConfig.
func (c *TopLevelConfig) ListVolumes(pool string) (map[string]*VolumeConfig, error) {
	poolPath := c.prefixed(rootVolume, pool)

	resp, err := c.etcdClient.Get(poolPath, true, true)
	if err != nil {
		return nil, err
	}

	configs := map[string]*VolumeConfig{}

	for _, node := range resp.Node.Nodes {
		if node.Value == "" {
			continue
		}

		config := &VolumeConfig{}
		if err := json.Unmarshal([]byte(node.Value), config); err != nil {
			return nil, err
		}

		key := strings.TrimPrefix(node.Key, poolPath)
		// trim leading slash
		configs[key[1:]] = config
	}

	return configs, nil
}

// ListPools returns an array with all the named pools the volmaster knows
// about.
func (c *TopLevelConfig) ListPools() ([]string, error) {
	resp, err := c.etcdClient.Get(c.prefixed(rootVolume), true, true)
	if err != nil {
		return nil, err
	}

	ret := []string{}

	for _, node := range resp.Node.Nodes {
		key := strings.TrimPrefix(node.Key, c.prefixed(rootVolume))
		// trim leading slash
		ret = append(ret, key[1:])
	}

	return ret, nil
}

// Validate validates a tenant configuration, returning error on any issue.
func (cfg *VolumeConfig) Validate() error {
	if cfg.Options.Size == 0 {
		return fmt.Errorf("Config for tenant has a zero size")
	}

	if cfg.Options.UseSnapshots && (cfg.Options.Snapshot.Frequency == "" || cfg.Options.Snapshot.Keep == 0) {
		return fmt.Errorf("Snapshots are configured but cannot be used due to blank settings")
	}

	return nil
}
