package types

import "fmt"

func NewParams(
	allowedPublishers []string,
	channels []string,
	maxArtifacts uint32,
	maxURLsPerArt uint32,
	maxSigsPerArt uint32,
	maxNotesLen uint32,
) Params {
	return Params{
		AllowedPublishers: allowedPublishers,
		Channels:          channels,
		MaxArtifacts:      maxArtifacts,
		MaxUrlsPerArt:     maxURLsPerArt,
		MaxSigsPerArt:     maxSigsPerArt,
		MaxNotesLen:       maxNotesLen,
	}
}

func DefaultParams() Params {
	return NewParams(
		[]string{},                 // no publishers by default
		[]string{"stable", "beta"}, // allowed channels
		8,                          // MaxArtifacts
		8,                          // MaxURLsPerArt
		4,                          // MaxSigsPerArt
		512,                        // MaxNotesLen
	)
}

func (p Params) Validate() error {
	if len(p.Channels) == 0 {
		return fmt.Errorf("channels must not be empty")
	}
	seen := map[string]struct{}{}
	for _, c := range p.Channels {
		if c == "" {
			return fmt.Errorf("channel must not be empty")
		}
		if _, ok := seen[c]; ok {
			return fmt.Errorf("duplicate channel: %s", c)
		}
		seen[c] = struct{}{}
	}
	if p.MaxArtifacts == 0 {
		return fmt.Errorf("max_artifacts must be > 0")
	}
	if p.MaxUrlsPerArt == 0 {
		return fmt.Errorf("max_urls_per_art must be > 0")
	}
	if p.MaxSigsPerArt == 0 {
		return fmt.Errorf("max_sigs_per_art must be > 0")
	}
	if p.MaxNotesLen == 0 {
		return fmt.Errorf("max_notes_len must be > 0")
	}
	return nil
}
