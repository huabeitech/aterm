package service

import "ssh-connect/internal/domain"

const MaxDirectoryNameLength = domain.MaxDirectoryNameLength

var (
	ErrAliasRequired                  = domain.ErrAliasRequired
	ErrAliasWhitespace                = domain.ErrAliasWhitespace
	ErrHostNameRequired               = domain.ErrHostNameRequired
	ErrPortInvalid                    = domain.ErrPortInvalid
	ErrDirectoryNameRequired          = domain.ErrDirectoryNameRequired
	ErrDirectoryNameTooLong           = domain.ErrDirectoryNameTooLong
	ErrDirectoryNameInvalidCharacters = domain.ErrDirectoryNameInvalidCharacters
	ErrDirectoryParentCycle           = domain.ErrDirectoryParentCycle
	ErrDirectoryNotEmpty              = domain.ErrDirectoryNotEmpty
)

func NormalizeHostInput(input HostInput) HostInput {
	return domain.NormalizeHostInput(input)
}

func ValidateHostInput(input HostInput) error {
	return domain.ValidateHostInput(input)
}

func NormalizeDirectoryInput(input DirectoryInput) DirectoryInput {
	return domain.NormalizeDirectoryInput(input)
}

func ValidateDirectoryInput(input DirectoryInput) error {
	return domain.ValidateDirectoryInput(input)
}
