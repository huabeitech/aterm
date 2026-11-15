package service

// Passwords are exact secrets, never trimmed. Length uses JavaScript UTF-16
// units so the shared Electron React form and the Go service enforce one rule.
func validEncryptionPassword(password string, minimum int) bool {
	length := 0
	for _, r := range password {
		length++
		if r > 0xffff {
			length++
		}
		if length > 4096 {
			return false
		}
	}
	return length >= minimum
}
