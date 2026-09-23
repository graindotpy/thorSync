package adapter

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"strings"

	"github.com/apgul/thorsync/internal/model"
)

const (
	pokemonGen3SaveSize       = 128 * 1024
	pokemonGen3SectorSize     = 4 * 1024
	pokemonGen3DataSize       = 0xF80
	pokemonGen3SectorsPerSlot = 14
	pokemonGen3Signature      = 0x08012025
	pokemonGen3AllSectors     = (1 << pokemonGen3SectorsPerSlot) - 1
)

type pokemonGen3CycleKey struct {
	slot    int
	counter uint32
}

type pokemonGen3Cycle struct {
	counter     uint32
	validMask   uint16
	invalidMask uint16
}

// ValidatePayload performs conservative, format-aware checks when a supported
// save structure can be identified. Unknown GBA saves remain byte-preserving
// and are not rejected merely because ThorSync cannot interpret them.
func ValidatePayload(platform model.Platform, data []byte) error {
	if platform != model.PlatformGBA || len(data) != pokemonGen3SaveSize {
		return nil
	}

	cycles := map[pokemonGen3CycleKey]*pokemonGen3Cycle{}
	recognizedSectors := 0
	for physicalSector := 0; physicalSector < pokemonGen3SectorsPerSlot*2; physicalSector++ {
		base := physicalSector * pokemonGen3SectorSize
		if binary.LittleEndian.Uint32(data[base+0xFF8:base+0xFFC]) != pokemonGen3Signature {
			continue
		}
		sectionID := binary.LittleEndian.Uint16(data[base+0xFF4 : base+0xFF6])
		if sectionID >= pokemonGen3SectorsPerSlot {
			continue
		}

		recognizedSectors++
		counter := binary.LittleEndian.Uint32(data[base+0xFFC : base+0x1000])
		key := pokemonGen3CycleKey{slot: physicalSector / pokemonGen3SectorsPerSlot, counter: counter}
		cycle := cycles[key]
		if cycle == nil {
			cycle = &pokemonGen3Cycle{counter: counter}
			cycles[key] = cycle
		}

		mask := uint16(1 << sectionID)
		storedChecksum := binary.LittleEndian.Uint16(data[base+0xFF6 : base+0xFF8])
		if pokemonGen3Checksum(data[base:base+pokemonGen3DataSize]) == storedChecksum {
			cycle.validMask |= mask
		} else {
			cycle.invalidMask |= mask
		}
	}
	if recognizedSectors == 0 {
		return nil
	}

	var latest *pokemonGen3Cycle
	var latestComplete *pokemonGen3Cycle
	for _, cycle := range cycles {
		if latest == nil || pokemonCounterAfter(cycle.counter, latest.counter) ||
			(cycle.counter == latest.counter && bits.OnesCount16(cycle.validMask) > bits.OnesCount16(latest.validMask)) {
			latest = cycle
		}
		if cycle.validMask == pokemonGen3AllSectors &&
			(latestComplete == nil || pokemonCounterAfter(cycle.counter, latestComplete.counter)) {
			latestComplete = cycle
		}
	}
	if latest == nil || latest.validMask == pokemonGen3AllSectors {
		return nil
	}

	missing := make([]string, 0, pokemonGen3SectorsPerSlot)
	invalid := make([]string, 0, pokemonGen3SectorsPerSlot)
	for sectionID := 0; sectionID < pokemonGen3SectorsPerSlot; sectionID++ {
		mask := uint16(1 << sectionID)
		if latest.validMask&mask == 0 {
			missing = append(missing, fmt.Sprintf("%d", sectionID))
		}
		if latest.invalidMask&mask != 0 {
			invalid = append(invalid, fmt.Sprintf("%d", sectionID))
		}
	}

	detail := fmt.Sprintf(
		"incomplete Pokémon Gen III save cycle %d: %d of %d sectors are valid; missing sectors %s",
		latest.counter,
		bits.OnesCount16(latest.validMask),
		pokemonGen3SectorsPerSlot,
		strings.Join(missing, ", "),
	)
	if len(invalid) > 0 {
		detail += "; checksum failures in sectors " + strings.Join(invalid, ", ")
	}
	if latestComplete != nil {
		detail += fmt.Sprintf("; previous complete cycle %d remains recoverable", latestComplete.counter)
	} else {
		detail += "; no complete fallback cycle was found"
	}
	return errors.New(detail)
}

func pokemonGen3Checksum(data []byte) uint16 {
	var checksum uint32
	for offset := 0; offset+4 <= len(data); offset += 4 {
		checksum += binary.LittleEndian.Uint32(data[offset : offset+4])
	}
	return uint16((checksum >> 16) + (checksum & 0xFFFF))
}

func pokemonCounterAfter(candidate, current uint32) bool {
	if candidate == current {
		return false
	}
	return candidate-current < 1<<31
}
