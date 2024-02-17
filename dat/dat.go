// Package dat is a parser for dat files, a binary database of games with
// metadata also used by RetroArch.
package dat

import (
	"encoding/xml"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"sync"
)

// DB is a database that contains many Dats, mapped to their system name
type DB map[string]Dat

// Dat is a list of the games of a system
type Dat struct {
	XMLName xml.Name `xml:"datafile"`
	Games   []Game   `xml:"game"`
}

// Game represents a game and can contain a list of ROMs
type Game struct {
	XMLName     xml.Name `xml:"game"`
	Name        string   `xml:"name,attr"`
	Description string   `xml:"description"` // The human readable name of the game
	ROMs        []ROM    `xml:"rom"`

	Path   string
	System string
}

// CRC is the CRC32 checksum of a ROM
type CRC uint32

// ROM can be a game file or part of a game
type ROM struct {
	XMLName xml.Name `xml:"rom"`
	Name    string   `xml:"name,attr"`
	CRC     CRC      `xml:"crc,attr"`
}

// UnmarshalXMLAttr is used to parse a hex number in string form to uint
func (s *CRC) UnmarshalXMLAttr(attr xml.Attr) error {
	u64, err := strconv.ParseUint(attr.Value, 16, 64)
	if err != nil {
		log.Println(err)
	} else {
		*s = CRC(uint32(u64))
	}
	return nil
}

// Parse parses a .dat file content and returns an array of Entries
func Parse(dat []byte) Dat {
	var output Dat

	err := xml.Unmarshal(dat, &output)
	if err != nil {
		log.Println(err)
	}

	return output
}

// FindByCRC loops over the Dats in the DB and concurrently matches CRC checksums.
func (db *DB) FindByCRC(romPath string, romName string, crc uint32, games chan (Game)) {
	var wg sync.WaitGroup
	wg.Add(len(*db))
	type SafeBool struct {
		mu    sync.Mutex
		found bool
	}
	game_found := SafeBool{found: false}
	// For every Dat in the DB
	for system, dat := range *db {
		go func(dat Dat, crc uint32, system string) {
			// For each game in the Dat
			for _, game := range dat.Games {
				if len(game.ROMs) == 0 {
					continue
				}
				// If the checksums match
				if crc == uint32(game.ROMs[0].CRC) {
					game.Path = romPath
					game.System = system
					games <- game
					game_found.mu.Lock()
					game_found.found = true
					game_found.mu.Unlock()
				}
			}
			wg.Done()
		}(dat, crc, system)
	}
	// Synchronize all the goroutines
	wg.Wait()
	if !game_found.found {
		fmt.Println(romName)
		db.FindByROMName(romPath, filepath.Base(romPath), crc, games)
	}
}

// FindByROMName loops over the Dats in the DB and concurrently matches ROM names.
// I'm going to update this to do fuzzy matching. To me that means:
//   - try to build a list with a mutex,
//   - if there is an exact name match use that
//   - otherwise at the end look through the potential matches with a few
//     adjustments for country codes (hoping for exact match)
//   - finally try to find a match without country code
//   - before failing
type SafeLookup struct {
	mu    sync.Mutex
	options []string
	found bool
}
game_found := SafeLookup{options: [], found: false}
func (db *DB) FindByROMName(romPath string, romName string, crc uint32, games chan (Game)) {
	var wg sync.WaitGroup
	wg.Add(len(*db))
	// For every Dat in the DB
	for system, dat := range *db {
		go func(dat Dat, crc uint32, system string) {
			// For each game in the Dat
			for _, game := range dat.Games {
				if len(game.ROMs) == 0 {
					continue
				}
				// If the checksums match
				for _, ROM := range game.ROMs {
					if romName == ROM.Name {
						game.Path = romPath
						game.System = system
						games <- game
						game_found.mu.Lock()
						game_found.found = true
						game_found.mu.Unlock()
					}
				}
			}
			wg.Done()
		}(dat, crc, system)
	}
	// Synchronize all the goroutines
	wg.Wait()
}
