package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateTopologyTree_Basic(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  api-gateway:
    image: nginx
    depends_on:
      - auth-service
      - product-service
  auth-service:
    image: node:18
    depends_on:
      - db
  product-service:
    image: python:3.11
    depends_on:
      - db
  db:
    image: postgres:15
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	tree, err := GenerateTopologyTree(dir)
	if err != nil {
		t.Fatalf("GenerateTopologyTree failed: %v", err)
	}

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}

	// Root should have children (networks)
	if len(tree.Children) == 0 {
		t.Error("Expected at least one network node in tree")
	}
}

func TestGenerateTopologyTree_CustomNetworks(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx
    networks:
      - frontend
      - backend
  api:
    image: node:18
    networks:
      - backend
  db:
    image: postgres
    networks:
      - backend
networks:
  frontend:
  backend:
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	tree, err := GenerateTopologyTree(dir)
	if err != nil {
		t.Fatalf("GenerateTopologyTree failed: %v", err)
	}

	// Should have 2 network groups: frontend and backend
	if len(tree.Children) != 2 {
		t.Errorf("Expected 2 network groups, got %d", len(tree.Children))
	}
}

func TestGenerateTopologyTree_MapStyleNetworks(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx
    networks:
      frontend:
        aliases:
          - web-alias
      backend:
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	tree, err := GenerateTopologyTree(dir)
	if err != nil {
		t.Fatalf("GenerateTopologyTree failed: %v", err)
	}

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}
	if len(tree.Children) != 2 {
		t.Errorf("Expected 2 network groups for map-style networks, got %d", len(tree.Children))
	}
}

func TestGenerateTopologyTree_DefaultNetwork(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx
  db:
    image: postgres
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	tree, err := GenerateTopologyTree(dir)
	if err != nil {
		t.Fatalf("GenerateTopologyTree failed: %v", err)
	}

	// Both services should be under "default" network
	if len(tree.Children) != 1 {
		t.Errorf("Expected 1 network group (default), got %d", len(tree.Children))
	}
}

func TestGenerateTopologyTree_NoFile(t *testing.T) {
	dir := t.TempDir()
	_, err := GenerateTopologyTree(dir)
	if err == nil {
		t.Error("Expected error for missing compose file")
	}
}

func TestGenerateTopologyTree_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("[invalid yaml{"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := GenerateTopologyTree(dir)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestGenerateTopologyTree_NoServices(t *testing.T) {
	dir := t.TempDir()
	compose := `version: "3"
networks:
  default:
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := GenerateTopologyTree(dir)
	if err == nil {
		t.Error("Expected error for compose file with no services")
	}
}

func TestGenerateTopologyTree_MapDependsOn(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx
    depends_on:
      db:
        condition: service_healthy
      cache:
        condition: service_started
  db:
    image: postgres
  cache:
    image: redis
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	tree, err := GenerateTopologyTree(dir)
	if err != nil {
		t.Fatalf("GenerateTopologyTree failed: %v", err)
	}
	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}
}
