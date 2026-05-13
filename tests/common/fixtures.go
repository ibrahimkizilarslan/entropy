package common

// ConfigFixtures contains common configuration YAML strings for testing.
type ConfigFixtures struct{}

// ValidChaosConfig returns a valid chaos configuration YAML string.
func (cf *ConfigFixtures) ValidChaosConfig() string {
	return `interval: 10
targets:
  - service-a
  - service-b
actions:
  - name: stop
  - name: restart
safety:
  max_down: 1
  cooldown: 30
  dry_run: false
`
}

// MinimalChaosConfig returns a minimal valid chaos configuration.
func (cf *ConfigFixtures) MinimalChaosConfig() string {
	return `interval: 5
targets:
  - test-service
actions:
  - name: pause
safety:
  max_down: 1
  cooldown: 10
  dry_run: true
`
}

// InvalidChaosConfig returns an invalid chaos configuration.
func (cf *ConfigFixtures) InvalidChaosConfig() string {
	return `invalid: yaml: content: [`
}

// NoTargetsConfig returns a config with no targets.
func (cf *ConfigFixtures) NoTargetsConfig() string {
	return `interval: 10
targets: []
actions:
  - name: stop
safety:
  max_down: 1
  cooldown: 30
  dry_run: false
`
}

// ZeroIntervalConfig returns a config with zero interval.
func (cf *ConfigFixtures) ZeroIntervalConfig() string {
	return `interval: 0
targets:
  - service-a
actions:
  - name: stop
safety:
  max_down: 1
  cooldown: 30
  dry_run: false
`
}

// ScenarioFixtures contains common scenario YAML strings for testing.
type ScenarioFixtures struct{}

// BasicScenario returns a basic scenario YAML string.
func (sf *ScenarioFixtures) BasicScenario() string {
	return `name: "Basic Test Scenario"
description: "A basic test scenario"
hypothesis: "System should recover after service failure"
steps:
  - probe:
      type: http
      url: "http://localhost:8080/health"
      expect_status: 200
  
  - inject:
      target: "test-service"
      action: stop
  
  - wait: 2s
  
  - probe:
      type: http
      url: "http://localhost:8080/health"
      expect_status: 503
`
}

// MultiStepScenario returns a scenario with multiple steps.
func (sf *ScenarioFixtures) MultiStepScenario() string {
	return `name: "Multi-Step Scenario"
description: "Test multiple chaos actions"
hypothesis: "System recovers from multiple failures"
steps:
  - inject:
      target: "service-a"
      action: pause
  
  - inject:
      target: "service-b"
      action:
        name: delay
        latency_ms: 500
        duration: 10
  
  - wait: 5s
  
  - probe:
      type: http
      url: "http://localhost:8080/status"
      expect_status: 200
`
}

// ComposeFixtures contains docker-compose related test fixtures.
type ComposeFixtures struct{}

// BasicDockerCompose returns a minimal docker-compose.yml content.
func (cf *ComposeFixtures) BasicDockerCompose() string {
	return `version: '3'
services:
  web:
    image: nginx:latest
  api:
    image: node:18-alpine
  db:
    image: postgres:15
`
}

// ComplexDockerCompose returns a more complex docker-compose.yml.
func (cf *ComposeFixtures) ComplexDockerCompose() string {
	return `version: '3.8'
services:
  api-gateway:
    image: nginx:latest
    ports:
      - "80:80"
  auth-service:
    image: node:18-alpine
    depends_on:
      - db
  product-service:
    image: python:3.11
  cart-service:
    image: python:3.11
  db:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: password
`
}
