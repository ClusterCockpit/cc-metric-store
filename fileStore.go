package main

/*

//MetricFile holds the state for a metric store file.
//It does not export any variable.
type FileStore struct {
	metrics    map[string]int
	numMetrics int
	size       int64
	root       string
}

func getFileName(tp string, ts string, start int64) string {

}

func newFileStore(root string, size int64, o []string) {
	var f FileStore
	f.root = root
	f.size = size

	for i, name := range o {
		f.metrics[name] = i * f.size * 8
	}
}

func openFile(fp string, hd *FileHeader) (f *File, err error) {
	f, err = os.OpenFile(file, os.O_WRONLY, 0644)

	if err != nil {
		return f, err
	}

}

func createFile(fp string) (f *File, err error) {
	f, err = os.Create(fp)

	if err != nil {
		return f, err
	}

}

func getFileHandle(file string, start int64) (f *File, err error) {
	f, err = os.OpenFile(file, os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return f, err
	}

	if _, err := f.Write([]byte("appended some data\n")); err != nil {
		f.Close() // ignore error; Write error takes precedence
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}

	return f
}
*/
