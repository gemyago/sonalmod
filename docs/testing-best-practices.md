# Testing Best Practices

## Core Principles

### Follow TDD
- Iterate with small chunks of logic/code
- Add stub implementation first if needed
- Write test to cover new logic or new behavior
- Run test to see if it fails
- Implement the minimal amount of code to make the test pass
- Repeat the process until your code is complete
- See [TDD Flow](../.context/tdd-flow.md) for more detailed guide

### Testing Philosophy
- **Focus on business logic** - Test the core functionality your code needs to provide
- **Avoid excessive tests** - Don't test scenarios that aren't relevant to your actual use cases
- **Avoid fragmenting one path into multiple tests** - Cover each unique behavior or execution path with a single comprehensive test
- **Test behavior, not implementation** - Focus on what the code does, not how it does it

### Keep It Simple
- **Pragmatic mocks** - Use simple mock implementations, avoid over-engineering
- **Minimal test setup** - Only setup what's necessary for the specific test case
- **Clear test names** - Use descriptive names that explain what behavior is being tested

### Don't Test the Framework
- **Skip infrastructure testing** - Don't test that logging works, that HTTP requests work, etc.
- **Trust the standard library** - Don't test Go's built-in functionality
- **Focus on your logic** - Test the decisions and transformations your code makes