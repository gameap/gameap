package domain

import (
	"maps"
	"strings"

	"github.com/gameap/gameap/pkg/validation"
	"github.com/pkg/errors"
)

// Limits mirroring the column widths of the dedicated_servers table; the HTTP
// handler enforces the same numbers.
const (
	NodeNameMaxLength     = 128
	NodeLocationMaxLength = 128
	NodeProviderMaxLength = 128
	NodePathMaxLength     = 512

	NodeMetadataMaxKeys        = 512
	NodeMetadataKeyMaxLength   = 255
	NodeMetadataValueMaxLength = 16 * 1024
)

var (
	ErrNodeNameRequired     = errors.New("name must not be empty")
	ErrNodeNameTooLong      = errors.New("name is too long")
	ErrNodeLocationRequired = errors.New("location must not be empty")
	ErrNodeLocationTooLong  = errors.New("location is too long")
	ErrNodeProviderTooLong  = errors.New("provider is too long")
	ErrNodeWorkPathRequired = errors.New("work_path must not be empty")
	ErrNodePathTooLong      = errors.New("path is too long")
	ErrNodeIPInvalid        = errors.New("ip is not a valid address or hostname")
	ErrNodeMetadataKeyEmpty = errors.New("metadata key must not be empty")
	ErrNodeMetadataTooLarge = errors.New("metadata is too large")
)

// NodePatch is a partial update of the operator-visible part of a node. It
// deliberately cannot express daemon credentials, certificates, the daemon
// address or the management scripts: callers that may only relabel a node
// (currently the gameap-nodes host library) apply changes through this type,
// so a widened field list is a conscious edit here rather than an oversight
// in a request mapping.
type NodePatch struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	Name         *string `json:"name,omitempty"`
	Location     *string `json:"location,omitempty"`
	Provider     *string `json:"provider,omitempty"`
	WorkPath     *string `json:"work_path,omitempty"`
	SteamcmdPath *string `json:"steamcmd_path,omitempty"`

	// IPs replaces the stored address list when non-empty.
	IPs []string `json:"ips,omitempty"`

	// Metadata is merged into the stored bag: listed keys are overwritten,
	// unlisted ones survive. Several actors (the admin UI, other plugins)
	// share the bag, so there is deliberately no wholesale replace.
	Metadata Metadata `json:"metadata,omitempty"`
	// RemoveMetadataKeys is applied after the merge.
	RemoveMetadataKeys []string `json:"remove_metadata_keys,omitempty"`
}

// Validate reports the first problem with the patch. Only fields that are
// present are checked — a nil pointer means "leave as is".
func (p *NodePatch) Validate() error {
	if err := p.validateStrings(); err != nil {
		return err
	}

	for _, ip := range p.IPs {
		if !validation.IsValidIPOrHostname(ip) {
			return errors.WithMessage(ErrNodeIPInvalid, ip)
		}
	}

	return p.validateMetadata()
}

func (p *NodePatch) validateStrings() error {
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			return ErrNodeNameRequired
		}
		if len(name) > NodeNameMaxLength {
			return ErrNodeNameTooLong
		}
	}

	if p.Location != nil {
		location := strings.TrimSpace(*p.Location)
		if location == "" {
			return ErrNodeLocationRequired
		}
		if len(location) > NodeLocationMaxLength {
			return ErrNodeLocationTooLong
		}
	}

	if p.Provider != nil && len(*p.Provider) > NodeProviderMaxLength {
		return ErrNodeProviderTooLong
	}

	if p.WorkPath != nil {
		workPath := strings.TrimSpace(*p.WorkPath)
		if workPath == "" {
			return ErrNodeWorkPathRequired
		}
		if len(workPath) > NodePathMaxLength {
			return ErrNodePathTooLong
		}
	}

	if p.SteamcmdPath != nil && len(*p.SteamcmdPath) > NodePathMaxLength {
		return ErrNodePathTooLong
	}

	return nil
}

func (p *NodePatch) validateMetadata() error {
	if len(p.Metadata) > NodeMetadataMaxKeys {
		return ErrNodeMetadataTooLarge
	}

	for key, value := range p.Metadata {
		if strings.TrimSpace(key) == "" {
			return ErrNodeMetadataKeyEmpty
		}
		if len(key) > NodeMetadataKeyMaxLength {
			return errors.WithMessage(ErrNodeMetadataTooLarge, "key "+key)
		}
		if str, ok := value.(string); ok && len(str) > NodeMetadataValueMaxLength {
			return errors.WithMessage(ErrNodeMetadataTooLarge, "value of "+key)
		}
	}

	for _, key := range p.RemoveMetadataKeys {
		if strings.TrimSpace(key) == "" {
			return ErrNodeMetadataKeyEmpty
		}
	}

	return nil
}

// ApplyTo writes the present fields onto the node. Metadata is merged and then
// the removal list is applied; an emptied bag becomes nil so the column stores
// NULL instead of "{}".
func (p *NodePatch) ApplyTo(node *Node) {
	if p.Enabled != nil {
		node.Enabled = *p.Enabled
	}
	if p.Name != nil {
		node.Name = strings.TrimSpace(*p.Name)
	}
	if p.Location != nil {
		node.Location = strings.TrimSpace(*p.Location)
	}
	if p.Provider != nil {
		node.Provider = p.Provider
	}
	if p.WorkPath != nil {
		node.WorkPath = strings.TrimSpace(*p.WorkPath)
	}
	if p.SteamcmdPath != nil {
		node.SteamcmdPath = p.SteamcmdPath
	}
	if len(p.IPs) > 0 {
		node.IPs = p.IPs
	}

	p.applyMetadataTo(node)
}

func (p *NodePatch) applyMetadataTo(node *Node) {
	if len(p.Metadata) == 0 && len(p.RemoveMetadataKeys) == 0 {
		return
	}

	merged := make(Metadata, len(node.Metadata)+len(p.Metadata))
	maps.Copy(merged, node.Metadata)
	maps.Copy(merged, p.Metadata)
	for _, key := range p.RemoveMetadataKeys {
		delete(merged, key)
	}

	if len(merged) == 0 {
		node.Metadata = nil

		return
	}

	node.Metadata = merged
}
