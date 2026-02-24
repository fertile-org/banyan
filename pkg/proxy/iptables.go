package proxy

// IPTables abstracts iptables operations for testability.
// The coreos/go-iptables *iptables.IPTables struct satisfies this interface.
type IPTables interface {
	NewChain(table, chain string) error
	ClearChain(table, chain string) error
	DeleteChain(table, chain string) error
	Append(table, chain string, rulespec ...string) error
	Insert(table, chain string, pos int, rulespec ...string) error
	Delete(table, chain string, rulespec ...string) error
	Exists(table, chain string, rulespec ...string) (bool, error)
	List(table, chain string) ([]string, error)
	ListChains(table string) ([]string, error)
}

// clearAndDeleteChain flushes and removes a chain in a single operation.
func clearAndDeleteChain(ipt IPTables, table, chain string) error {
	if err := ipt.ClearChain(table, chain); err != nil {
		return err
	}
	return ipt.DeleteChain(table, chain)
}
