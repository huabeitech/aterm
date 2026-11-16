package service

const CloudBackupDirectoryName = "aTerm"

func IsMacAppStoreBuild() bool { return macAppStoreBuild }

// DataDirectoryName keeps development files and credentials in the same scope.
func DataDirectoryName() string {
	if developmentBuild {
		return "TermSSH-dev"
	}
	return "TermSSH"
}
