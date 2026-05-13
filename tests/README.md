# Entropy Test Suite

This directory contains all test files for the Entropy chaos engineering toolkit. The tests are organized by module for better maintainability and clarity.

## Directory Structure

```
tests/
├── cli/              # CLI command tests
├── engine/           # Chaos engine and action handler tests
├── worker/           # Worker daemon and background process tests
├── common/           # Shared test utilities and fixtures
└── README.md         # This file
```

## Module Coverage

### tests/cli/
Tests for CLI commands and user-facing features:
- `root_test.go` - Root command structure and subcommands
- `chaos_test.go` - Chaos control commands (start, stop, status, logs, cleanup)
- `init_test.go` - Initialization and docker-compose discovery
- `scenario_test.go` - Scenario command and execution

### tests/engine/
Tests for the chaos engine core functionality:
- `actions_test.go` - Chaos action handlers and dispatch
- `chaos_engine_test.go` - Engine initialization and action availability

### tests/worker/
Tests for background worker processes:
- `daemon_test.go` - Daemon initialization and configuration handling

### tests/common/
Shared testing utilities and test data:
- `fixture.go` - TestFixture class for managing temporary test directories
- `fixtures.go` - Reusable test data generators (configs, scenarios, docker-compose files)

## Running Tests

### Run All Tests
```bash
make test
```

### Run Tests by Module
```bash
make test-cli
make test-engine
make test-worker
```

### Run with Coverage
```bash
make test-coverage
```

### Run Specific Test
```bash
go test ./tests/cli -v -run TestRootCommand
```

## Test Coverage Goals

Current coverage targets:
- **CLI Module**: 80%+ - Command parsing, config generation, safety checks
- **Engine Module**: 85%+ - Action dispatch, chaos injection logic
- **Worker Module**: 75%+ - Daemon lifecycle, config loading, safety parameters
- **Config Module**: 90%+ - YAML parsing, validation, defaults
- **Utils Module**: 80%+ - State management, logging, utilities

## Writing New Tests

### Using Fixtures

```go
package cli_test

import (
	"testing"
	"github.com/ibrahimkizilarslan/entropy/tests/common"
)

func TestMyFeature(t *testing.T) {
	// Create a test fixture
	fixture := common.NewTestFixture(t)
	
	// Create test files
	configPath := fixture.CreateFile("chaos.yaml", 
		[]byte(common.ConfigFixtures{}.ValidChaosConfig()))
	
	// Your test code here
	if !fixture.FileExists("chaos.yaml") {
		t.Fatal("Config file not created")
	}
}
```

### Using Config Fixtures

```go
func TestConfigValidation(t *testing.T) {
	fixtures := common.ConfigFixtures{}
	validConfig := fixtures.ValidChaosConfig()
	invalidConfig := fixtures.InvalidChaosConfig()
	
	// Use in your tests
}
```

## Test Conventions

1. **Table-driven tests**: Use tables for multiple test cases
2. **Temp directories**: Always use `t.TempDir()` or fixtures for file operations
3. **Error handling**: Test both success and failure paths
4. **Named parameters**: Use struct literals for clarity
5. **Cleanup**: Fixtures automatically clean up temp directories

## Continuous Integration

Tests are run on every commit via CI/CD pipeline:
- All tests must pass before merging
- Coverage reports are generated and tracked
- Tests run on Linux, macOS, and Windows

## Debugging Tests

### Verbose Output
```bash
go test ./tests/cli -v
```

### Run Specific Test
```bash
go test ./tests/cli -v -run TestStartCmdFlags
```

### Print Test Coverage
```bash
go test ./tests/cli -cover
```

### Interactive Debugging
```bash
dlv test ./tests/cli
```

## Common Issues

### Import Errors
Ensure imports use the full path: `github.com/ibrahimkizilarslan/entropy/pkg/config`

### Package Not Found
Tests are in `tests/` directory but import from `pkg/`. Run tests with:
```bash
go test ./tests/...
```

### Temp Directory Cleanup
The `TestFixture` automatically manages cleanup. Don't manually create temp dirs in new code.

## Contributing

When adding new features:
1. Write tests in the appropriate module subdirectory
2. Update this README if adding new test categories
3. Aim for >80% code coverage
4. Use the provided fixtures for consistency
5. Follow Go testing conventions (Table-driven, clear names, etc.)

## Resources

- [Go Testing Package](https://golang.org/pkg/testing/)
- [Testing Best Practices](https://golang.org/doc/effective_go#testing)
- [Table-Driven Tests](https://github.com/golang/go/wiki/TableDrivenTests)
