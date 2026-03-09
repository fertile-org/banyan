package proxy

// NoopIPTables is a no-op IPTables implementation for testing.
// All operations succeed silently without touching real iptables.
type NoopIPTables struct{}

// NewNoopIPTables creates a new NoopIPTables instance.
func NewNoopIPTables() *NoopIPTables { return &NoopIPTables{} }

func (n *NoopIPTables) NewChain(table, chain string) error                            { return nil }
func (n *NoopIPTables) ClearChain(table, chain string) error                          { return nil }
func (n *NoopIPTables) DeleteChain(table, chain string) error                         { return nil }
func (n *NoopIPTables) Append(table, chain string, rulespec ...string) error          { return nil }
func (n *NoopIPTables) Insert(table, chain string, pos int, rulespec ...string) error { return nil }
func (n *NoopIPTables) Delete(table, chain string, rulespec ...string) error          { return nil }
func (n *NoopIPTables) Exists(table, chain string, rulespec ...string) (bool, error) {
	return false, nil
}
func (n *NoopIPTables) List(table, chain string) ([]string, error) { return nil, nil }
func (n *NoopIPTables) ListChains(table string) ([]string, error)  { return nil, nil }
