package storage

import (
	"net/url"
	"strings"

	"github.com/brimdata/zed"
	"github.com/brimdata/zed/zson"
)

type URI url.URL

// ParseURI parses the path using `url.Parse`. If the provided uri does not
// contain a scheme, the scheme is set to file. Relative paths are
// treated as files and resolved as absolute paths using filepath.Abs.
// If path is an empty, a pointer to zero-valued URI is returned.
func ParseURI(path string) (*URI, error) {
	if path == "" {
		return &URI{}, nil
	}
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	if !knownScheme(Scheme(u.Scheme)) {
		// If we don't know the scheme, either it's empty string,
		// implying a file, or it's a file path with a colon embedded,
		// so we parse it either way as a file.
		return parseBarePath(path)
	}
	return (*URI)(u), nil
}

func MustParseURI(path string) *URI {
	u, err := ParseURI(path)
	if err != nil {
		panic(err)
	}
	return u
}

func (u URI) String() string {
	return (*url.URL)(&u).String()
}

func (u *URI) HasScheme(s Scheme) bool {
	return Scheme(u.Scheme) == s
}

func (p *URI) AppendPath(elem ...string) *URI {
	u := *p
	for _, el := range elem {
		u.Path = u.Path + "/" + el
	}
	return &u
}

func (u *URI) RelPath(target URI) string {
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return strings.TrimPrefix(target.Path, u.Path)
}

func (u *URI) IsZero() bool {
	return *u == URI{}
}

func (u *URI) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

func (u *URI) UnmarshalText(b []byte) error {
	uri, err := ParseURI(string(b))
	if err != nil {
		return err
	}
	*u = *uri
	return nil
}

func (u *URI) MarshalZNG(mc *zson.MarshalZNGContext) (zed.Type, error) {
	return mc.MarshalValue(u.String())
}
