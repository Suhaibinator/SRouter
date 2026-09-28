package scontext

// Param is one path parameter captured by a route pattern such as ":id" or
// "*path".
type Param struct {
	Key   string
	Value string
}

// Params holds the path parameters captured for a matched route, in the order
// they appear in the route pattern.
type Params []Param

// ByName returns the value of the first parameter named name, or "" when no
// such parameter exists.
func (ps Params) ByName(name string) string {
	value, _ := ps.Get(name)
	return value
}

// Get returns the value of the first parameter named name and whether it
// exists. It distinguishes a missing parameter from an empty value.
func (ps Params) Get(name string) (string, bool) {
	for _, p := range ps {
		if p.Key == name {
			return p.Value, true
		}
	}
	return "", false
}
