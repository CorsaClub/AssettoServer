// Package geoip provides geolocation services for player IP addresses.
package geoip

import (
	"errors"
	"net"
	"os"
	"sync"

	"metrics/config"
	"metrics/types"
	"metrics/utils"

	"github.com/oschwald/geoip2-golang"
)

// GeoIPService provides geolocation services for IP addresses.
type GeoIPService struct {
	db          *geoip2.Reader
	dbPath      string
	initialized bool
	mu          sync.RWMutex
}

// NewGeoIPService creates a new GeoIPService.
func NewGeoIPService(dbPath string) *GeoIPService {
	return &GeoIPService{
		dbPath: dbPath,
	}
}

// Initialize initializes the GeoIP database.
func (g *GeoIPService) Initialize() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.initialized {
		return nil
	}

	// Check if the database file exists
	if _, err := os.Stat(g.dbPath); os.IsNotExist(err) {
		return errors.New("GeoIP database file does not exist: " + g.dbPath)
	}

	// Open the database
	db, err := geoip2.Open(g.dbPath)
	if err != nil {
		return err
	}

	g.db = db
	g.initialized = true
	utils.LogInfo("GeoIP database initialized: %s", g.dbPath)
	return nil
}

// Close closes the GeoIP database.
func (g *GeoIPService) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.initialized || g.db == nil {
		return nil
	}

	err := g.db.Close()
	g.initialized = false
	g.db = nil
	return err
}

// LookupIP looks up geolocation information for an IP address.
func (g *GeoIPService) LookupIP(ipStr string) (*types.GeoLocation, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.initialized || g.db == nil {
		return nil, errors.New("GeoIP database not initialized")
	}

	// Parse the IP address
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, errors.New("invalid IP address: " + ipStr)
	}

	// Look up the IP address
	record, err := g.db.City(ip)
	if err != nil {
		return nil, err
	}

	// Create a GeoLocation object
	geo := &types.GeoLocation{
		IP:          ipStr,
		Country:     record.Country.Names["en"],
		CountryCode: record.Country.IsoCode,
		City:        record.City.Names["en"],
		Latitude:    record.Location.Latitude,
		Longitude:   record.Location.Longitude,
	}

	return geo, nil
}

// EnrichPlayerWithGeoIP enriches a player with geolocation information.
func (g *GeoIPService) EnrichPlayerWithGeoIP(player *types.Player) error {
	if player.IP == "" {
		return errors.New("player IP address is empty")
	}

	geo, err := g.LookupIP(player.IP)
	if err != nil {
		return err
	}

	player.Country = geo.Country
	player.CountryCode = geo.CountryCode
	player.City = geo.City
	player.Latitude = geo.Latitude
	player.Longitude = geo.Longitude

	return nil
}

// InitGeoIPService initializes the GeoIP service from the configuration.
func InitGeoIPService(cfg *config.GeoIPConfig) (*GeoIPService, error) {
	if !cfg.Enabled {
		utils.LogInfo("GeoIP service is disabled")
		return nil, nil
	}

	if cfg.DatabasePath == "" {
		return nil, errors.New("GeoIP database path is not configured")
	}

	service := NewGeoIPService(cfg.DatabasePath)
	if err := service.Initialize(); err != nil {
		return nil, err
	}

	utils.LogInfo("GeoIP service initialized successfully")
	return service, nil
}
