package fileManagerTypes

type WriteArgs struct {
	Content string
	Pos     int
	From    int
}

type ReadArgs struct {
	Len int
	Pos int
}

type UpdateArgs struct {
	Content string
	Pos     int
	From    int
}

type ReplyType struct {
	Err  int
	Data []byte
}
