package auth

const (
	EncryptionServiceID    = 2
	DataIntegrityServiceID = 3
)

func breakUpVersion(fullVersion uint32) (version, update int) {
	version = int(fullVersion >> 24)
	update = int(fullVersion & 0xFF000 >> 12)
	return
}

func isNew(fullVersion uint32) bool {
	version, update := breakUpVersion(fullVersion)
	return version >= 23 || update == 1
}
