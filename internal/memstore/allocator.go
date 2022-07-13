package memstore

func RequestBytes(len int) []byte {
	// TODO: Use mmap etc...!

	return make([]byte, len)
}
