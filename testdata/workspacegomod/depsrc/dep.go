package dep

// Answer returns a constant. It exists so workspace modules have a real
// symbol to import.
func Answer() int {
	return 42
}
