package types

// Object names the user defined type of a bind, for a value whose Go type
// does not name it, such as a collection of a basic type.
type Object struct {
	// Name is the name of the type, as RegisterType registered it.
	Name string
	// Value is the value, or a pointer to the destination of an OUT bind.
	Value any
}
