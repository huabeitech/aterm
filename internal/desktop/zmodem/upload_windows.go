package zmodem

import "os"

func openUpload(path string) (*os.File, error) { return os.Open(path) }
