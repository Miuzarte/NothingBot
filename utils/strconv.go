package utils

// ParseBool extends [strconv.ParseBool] with y/n
func ParseBool(str string) (b bool, ok bool) {
	switch str {
	case "1", "t", "T", "y", "Y", "true", "TRUE", "True", "yes", "YES", "Yes":
		return true, true
	case "0", "f", "F", "n", "N", "false", "FALSE", "False", "no", "NO", "No":
		return false, true
	}
	return false, false
}
