package gincage

import "errors"

var (
	ErrNoTokensAwailable  = errors.New("no tokens awailable in bucket")
	ErrBadSyntaxInStorage = errors.New("bad syntax in storage")
)
