package prompt

// Option は、選択画面の 1 つの候補。Disabled が空でなければ、それを選べない理由として示し、選ばせない。
type Option struct {
	Label    string
	Disabled string
}

// Choose は、title と候補を示して 1 つを選ばせ、選んだ候補の位置を返す。
// 利用者が中止したら ErrCanceled を返す。端末で対話できるときだけ使う。
type Choose func(title string, options []Option) (int, error)
