## Use this guide when coding in golang

## Key considerations

- **Consumer-Defined interface** All components should follow "accept interface and return struct" principle for dependencies:
  - Component dependencies (services, repositories, etc.) should be accepted as interfaces (for flexibility and testability)
  - Return types should be concrete structs (for clarity and avoiding unnecessary abstraction)
  - Strong justification is required to deviate from this pattern
- Define interfaces next to consumer by default (in a same file). Move to a separate if getting bigger or used by multiple consumers in the same package.
- Use log/slog, always provided as dependency, no globals.
- Wrap errors with `fmt.Errorf("<something>: %w", err)`

### Testing Style and Patterns

More detailed testing best practices are in [testing-best-practices.md](./testing-best-practices.md). Common principles:
- Define tests in same package
- Prefer a single top-level test function per component, with nested `t.Run` blocks organizing tests by method and their scenarios.
- Avoid static variables shared across tests
- Use makeMockDeps to initialize dependencies, no inline or repeated setup
- Use random data when possible, use faker (github.com/jaswdr/faker/v2)
- Don't pollute testing namespace - if helper functions are only used within one test, nest them inside that test function
- Compare entire structs when possible instead of individual fields (e.g `assert.Equal(t, expectedUser, actualUser)`)
- Use require.Error or require.ErrorIs when asserting errors
- Use `t.Context()` instead `context.Background()` OR `context.TODO()` in tests
- Use factory functions to create reusable random data
- Follow [mockery](../.context/mockery.md) for defining and generating mocks