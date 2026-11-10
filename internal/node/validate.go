package node

import "example.com/go-block-lattice/internal/protocol"

// ValidateFiles reads startup inputs without opening storage or listeners.
func (c Config) ValidateFiles() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if _, err := protocol.LoadGenesis(c.Genesis); err != nil {
		return err
	}
	key, err := loadRepresentative(c.RepresentativeKey)
	clear(key)
	return err
}
