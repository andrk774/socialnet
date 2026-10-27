package config

import (
	"strings"
	"testing"
)

func base(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x@db/socialnet")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
}

func TestLoadFull(t *testing.T) {
	base(t)
	for k, v := range map[string]string{
		"S3_ENDPOINT": "http://minio:9000/", "S3_BUCKET": "media", "S3_ACCESS_KEY": "a", "S3_SECRET_KEY": "b",
		"BACKUP_S3": "true", "BACKUP_ENC_KEY": strings.Repeat("ab", 32), "BACKUP_HOUR": "4",
		"REDIS_URL": "redis://redis:6379/0", "YOOKASSA_SHOP_ID": "1", "YOOKASSA_SECRET_KEY": "k", "YOOKASSA_VAT_CODE": "11",
		"MEDIA_CDN_URL": "https://cdn.example/",
	} {
		t.Setenv(k, v)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.S3Endpoint != "http://minio:9000" || !c.BackupS3 || len(c.BackupKey) != 32 || c.BackupHour != 4 ||
		c.YooKassaVATCode != 11 || c.MediaCDNURL != "https://cdn.example" || c.RedisURL == "" || !c.YooKassaCheckIP {
		t.Fatalf("%+v", c)
	}
}

func TestLoadErrors(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"backup s3 without s3": {"BACKUP_S3": "true"},
		"short backup key":     {"BACKUP_ENC_KEY": "abcd"},
		"bad vat":              {"YOOKASSA_VAT_CODE": "13"},
		"shop without secret":  {"YOOKASSA_SHOP_ID": "1"},
		"s3 without bucket":    {"S3_ENDPOINT": "http://minio:9000"},
		"bad backup hour":      {"BACKUP_HOUR": "24"},
		"tile url not http":    {"TILE_URL": "file:///etc/{z}/{x}/{y}"},
		"tile url no xyz":      {"TILE_URL": "https://tiles.example/map.png"},
		"bad tile zoom":        {"TILE_MAX_ZOOM": "25"},
		"offsite without key":  {"BACKUP_S3_ENDPOINT": "https://s3.example", "BACKUP_S3_BUCKET": "b", "BACKUP_S3_ACCESS_KEY": "a", "BACKUP_S3_SECRET_KEY": "s"},
		"offsite no bucket":    {"BACKUP_S3_ENDPOINT": "https://s3.example", "BACKUP_ENC_KEY": strings.Repeat("ab", 32)},
	} {
		t.Run(name, func(t *testing.T) {
			base(t)
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestTileConfig(t *testing.T) {
	base(t)
	c, err := Load()
	if err != nil || c.TileURL != "https://tile.openstreetmap.org/{z}/{x}/{y}.png" || c.TileMaxZoom != 17 || c.TileAttribution == "" {
		t.Fatalf("defaults: %v %+v", err, c)
	}
	t.Setenv("TILE_URL", "") // задано пустым — карта выключена
	if c, err = Load(); err != nil || c.TileURL != "" {
		t.Fatalf("disabled: %v %q", err, c.TileURL)
	}
}
