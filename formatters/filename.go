package formatters

import (
	"github.com/sblinch/BookBrowser/booklist"
	"strings"
	"path"
	"path/filepath"
)

type FilenameFormatter func(filename string, b *booklist.Book)

func uniqueStrings(sl []string) []string {
	seen := make(map[string]struct{}, len(sl))
	res := make([]string, 0, len(sl))
	for _, s := range sl {
		if _, exists := seen[s]; !exists {
			res = append(res, s)
			seen[s] = struct{}{}
		}
	}

	return res
}

var FilenameFormatters = map[string]FilenameFormatter{
	// Extracts author name from folder, book title from filename.
	// May be enabled if PDFs are organized by folders as in: /foo/bar/Author Name/Book Title.pdf
	"authorfolders": func(filename string, book *booklist.Book) {
		parent := filepath.Dir(filename)
		parentName := filepath.Base(parent)

		filename = filepath.Base(filename)
		filename = strings.TrimSuffix(filename, path.Ext(filename))

		book.Author = &booklist.Author{Name: parentName}
		book.Title = filename
	},

	// Splits the filename on " - " and treats the first segment as the author, and the remaining ones as the title.
	// Used with filenames such as: /foo/bar/baz/Author Name - Book Title.pdf
	"dashes": func(filename string, book *booklist.Book) {
		filename = filepath.Base(filename)
		filename = strings.TrimSuffix(filename, path.Ext(filename))

		pieces := strings.Split(filename, " - ")
		if len(pieces) <= 1 {
			return
		}
		pieces = uniqueStrings(pieces)
		if len(pieces) == 2 {
			book.Author = &booklist.Author{Name: pieces[0]}
			book.Title = pieces[1]
		} else if len(pieces) > 2 {
			book.Author = &booklist.Author{Name: pieces[0]}
			book.Title = strings.Join(pieces[1:], " - ")
		}
	},

	// Last resort; simply trims the file extension from the filename and uses it verbatim as the title.
	"titleonly": func(filename string, book *booklist.Book) {
		filename = filepath.Base(filename)
		filename = strings.TrimSuffix(filename, path.Ext(filename))

		if (book.Author == nil || book.Author.Name == "") && book.Title == "" {
			book.Title = filename
		}
	},
}

var EnabledFilenameFormatters = []string{
	"dashes", "titleonly",
}
