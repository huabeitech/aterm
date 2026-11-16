package service

const appName = "aTerm"

// CloudBackupDirectoryName is shared by all distributions.
const CloudBackupDirectoryName = appName

func IsMacAppStoreBuild() bool { return macAppStoreBuild }

// DataDirectoryName separates local development files from production files.
func DataDirectoryName() string {
	if developmentBuild {
		return appName + "-dev"
	}
	return appName
}
