package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"time"

	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/zstd"
	"github.com/oNaiPs/zipclick/internal/deflate64"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
	"golang.org/x/text/encoding/charmap"
)

// zipSource reads a zip through archive/zip, which does the parsing; members
// that are encrypted, or compressed with something other than Deflate, are
// decrypted and decompressed here from their raw bytes.
type zipSource struct {
	r        *zip.Reader
	f        *os.File
	i        int
	password string
	oem      *charmap.Charmap
}

func openZip(path, password string, p *Progress) (src source, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if src == nil {
			f.Close()
		}
	}()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// The file itself, not a read-ahead cache: members are read by several
	// goroutines at once (pread is fine with that), each through its own
	// buffer, set up in openZipFile and by the decompressor below.
	r, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return nil, err
	}
	r.RegisterDecompressor(zip.Deflate, func(r io.Reader) io.ReadCloser {
		return flate.NewReader(bufio.NewReaderSize(r, readAheadSize))
	})
	var total int64
	encrypted := false
	for _, f := range r.File {
		total += int64(f.UncompressedSize64)
		encrypted = encrypted || f.Flags&0x1 != 0
	}
	if encrypted && password == "" {
		return nil, ErrEncrypted
	}
	p.setTotal(total)
	return &zipSource{r: r, f: f, password: password, oem: oemDecoder()}, nil
}

func (s *zipSource) next() (*member, error) {
	if s.i >= len(s.r.File) {
		return nil, io.EOF
	}
	f := s.r.File[s.i]
	s.i++
	mode := f.Mode()
	return &member{
		name:  decodeName(f.Name, f.NonUTF8, s.oem),
		dir:   mode.IsDir(),
		skip:  mode&os.ModeSymlink != 0, // a link pointing outside the destination is not worth the risk
		exec:  mode.Perm()&0o100 != 0,
		size:  int64(f.UncompressedSize64),
		mtime: modTime(f),
		open:  func() (io.ReadCloser, error) { return openZipFile(f, s.password, s.oem) },
	}, nil
}

func (s *zipSource) countsInput() bool { return false }
func (s *zipSource) parallel() bool    { return true }
func (s *zipSource) Close() error      { return s.f.Close() }

// The compression methods ZipClick reads: Store and Deflate, which archive/zip
// does itself unless the member is encrypted, and the others zips in the wild
// use. Deflate64 (9) is what Explorer picks for large files.
var zipMethods = map[uint16]func(r io.Reader, f *zip.File) (io.ReadCloser, error){
	zip.Store:   func(r io.Reader, _ *zip.File) (io.ReadCloser, error) { return io.NopCloser(r), nil },
	zip.Deflate: func(r io.Reader, _ *zip.File) (io.ReadCloser, error) { return flate.NewReader(r), nil },
	9:           func(r io.Reader, _ *zip.File) (io.ReadCloser, error) { return deflate64.NewReader(r), nil },
	12:          func(r io.Reader, _ *zip.File) (io.ReadCloser, error) { return io.NopCloser(bzip2.NewReader(r)), nil },
	14:          openZipLZMA,
	93: func(r io.Reader, _ *zip.File) (io.ReadCloser, error) {
		z, err := zstd.NewReader(r, zstd.WithDecoderLowmem(true))
		if err != nil {
			return nil, err
		}
		return z.IOReadCloser(), nil
	},
	95: func(r io.Reader, _ *zip.File) (io.ReadCloser, error) {
		x, err := xz.NewReader(r)
		return io.NopCloser(x), err
	},
}

const methodAES = 99

// maxLZMADict bounds the LZMA dictionary a zip member may ask for.
const maxLZMADict = 256 << 20

func openZipFile(f *zip.File, password string, oem *charmap.Charmap) (io.ReadCloser, error) {
	encrypted := f.Flags&0x1 != 0
	method := f.Method
	var ae *aesExtra
	if method == methodAES {
		if ae = findAESExtra(f.Extra); ae == nil {
			return nil, &MethodError{method}
		}
		method = ae.method
	}
	if !encrypted && (method == zip.Store || method == zip.Deflate) {
		rc, err := f.Open()
		if errors.Is(err, zip.ErrAlgorithm) {
			return nil, &MethodError{f.Method}
		}
		return rc, err
	}
	decompress, ok := zipMethods[method]
	if !ok {
		return nil, &MethodError{method}
	}
	rawSection, err := f.OpenRaw()
	if err != nil {
		return nil, err
	}
	raw := bufio.NewReaderSize(rawSection, readAheadSize)
	var r io.Reader = raw
	var auth func() error
	switch {
	case ae != nil:
		r, auth, err = newAESReader(raw, int64(f.CompressedSize64), password, ae.strength)
	case encrypted:
		r, err = newZipCryptoReader(raw, f, password, oem)
	}
	if err != nil {
		return nil, err
	}
	rc, err := decompress(r, f)
	if err != nil {
		return nil, err
	}
	// AE-2 leaves the CRC out on purpose (it would leak a little about the
	// contents); its authentication code does that job.
	checkCRC := ae == nil || ae.version == 1
	return &checkedReader{rc: rc, crc: crc32.NewIEEE(), want: f.CRC32, checkCRC: checkCRC, auth: auth, encrypted: encrypted}, nil
}

// checkedReader verifies a member once it has been read to the end. A wrong
// password that slipped past the one-byte ZipCrypto check shows up here.
type checkedReader struct {
	rc        io.ReadCloser
	crc       hash.Hash32
	want      uint32
	checkCRC  bool
	auth      func() error
	encrypted bool
}

func (c *checkedReader) Read(b []byte) (int, error) {
	n, err := c.rc.Read(b)
	c.crc.Write(b[:n])
	if err == io.EOF {
		if c.auth != nil {
			if aerr := c.auth(); aerr != nil {
				return n, aerr
			}
		}
		if c.checkCRC && c.crc.Sum32() != c.want {
			if c.encrypted {
				return n, ErrBadPassword
			}
			return n, zip.ErrChecksum
		}
	} else if err != nil && c.encrypted && !errors.Is(err, ErrCancelled) {
		// Garbage in from a wrong key makes the decompressor fail first.
		return n, ErrBadPassword
	}
	return n, err
}

func (c *checkedReader) Close() error { return c.rc.Close() }

// openZipLZMA reads method 14: a 4-byte LZMA SDK version and properties
// length, the 5 properties bytes, then the raw stream. The lzma package wants
// the .lzma file header instead, which is those properties and the size.
func openZipLZMA(r io.Reader, f *zip.File) (io.ReadCloser, error) {
	var hdr [9]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	var lz [13]byte
	copy(lz[:5], hdr[4:9])
	size := f.UncompressedSize64
	if f.Flags&0x2 != 0 { // the stream ends with a marker
		size = ^uint64(0)
	}
	binary.LittleEndian.PutUint64(lz[5:], size)
	// The dictionary is allocated up front at the size the header asks for,
	// up to 2 GiB; zips never use more than 64 MiB, so a bigger one is a
	// crafted header, not an archive.
	lr, err := lzma.ReaderConfig{DictCap: maxLZMADict}.NewReader(io.MultiReader(bytes.NewReader(lz[:]), r))
	return io.NopCloser(lr), err
}

// ZipCrypto, the original PKWARE stream cipher. Weak, but still what most
// tools write unless asked for AES.

type zipCryptoReader struct {
	r    io.Reader
	keys [3]uint32
}

func newZipCryptoReader(r io.Reader, f *zip.File, password string, oem *charmap.Charmap) (io.Reader, error) {
	var header [12]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	// The last header byte repeats part of the CRC, or of the time when the
	// CRC came after the data. Only one byte, so a wrong password gets past
	// this one time in 256 and fails the CRC instead.
	check := byte(f.CRC32 >> 24)
	if f.Flags&0x8 != 0 {
		check = byte(f.ModifiedTime >> 8)
	}
	// Older tools encoded the password in the OEM code page, newer ones in
	// UTF-8; they only differ for accented passwords.
	candidates := [][]byte{[]byte(password)}
	if s, err := oem.NewEncoder().String(password); err == nil && s != password {
		candidates = append(candidates, []byte(s))
	}
	for _, pw := range candidates {
		z := &zipCryptoReader{r: r}
		z.init(pw)
		h := header
		z.decrypt(h[:])
		if h[11] == check {
			return z, nil
		}
	}
	return nil, ErrBadPassword
}

func (z *zipCryptoReader) init(password []byte) {
	z.keys = [3]uint32{0x12345678, 0x23456789, 0x34567890}
	for _, b := range password {
		z.update(b)
	}
}

func (z *zipCryptoReader) update(b byte) {
	z.keys[0] = crc32Update(z.keys[0], b)
	z.keys[1] = (z.keys[1]+z.keys[0]&0xff)*134775813 + 1
	z.keys[2] = crc32Update(z.keys[2], byte(z.keys[1]>>24))
}

func (z *zipCryptoReader) decrypt(b []byte) {
	for i := range b {
		t := uint16(z.keys[2] | 2)
		b[i] ^= byte(uint32(t) * uint32(t^1) >> 8)
		z.update(b[i])
	}
}

func (z *zipCryptoReader) Read(b []byte) (int, error) {
	n, err := z.r.Read(b)
	z.decrypt(b[:n])
	return n, err
}

func crc32Update(crc uint32, b byte) uint32 {
	return crc32.IEEETable[byte(crc)^b] ^ crc>>8
}

// WinZip AES (what 7-Zip, WinRAR and WinZip write when asked for AES):
// PBKDF2-SHA1 keys, AES in counter mode with a little-endian counter, and an
// HMAC-SHA1 over the encrypted data.

type aesExtra struct {
	version  uint16
	strength byte // 1, 2, 3: AES-128, -192, -256
	method   uint16
}

func findAESExtra(extra []byte) *aesExtra {
	for len(extra) >= 4 {
		tag := binary.LittleEndian.Uint16(extra)
		size := int(binary.LittleEndian.Uint16(extra[2:]))
		if 4+size > len(extra) {
			break
		}
		if tag == 0x9901 && size >= 7 {
			d := extra[4:]
			return &aesExtra{version: binary.LittleEndian.Uint16(d), strength: d[4], method: binary.LittleEndian.Uint16(d[5:])}
		}
		extra = extra[4+size:]
	}
	return nil
}

func newAESReader(raw io.Reader, size int64, password string, strength byte) (io.Reader, func() error, error) {
	if strength < 1 || strength > 3 {
		return nil, nil, &MethodError{methodAES}
	}
	keyLen := 8 + 8*int(strength) // 16, 24, 32
	saltLen := keyLen / 2
	salt := make([]byte, saltLen+2)
	if _, err := io.ReadFull(raw, salt); err != nil {
		return nil, nil, err
	}
	keys, err := pbkdf2.Key(sha1.New, password, salt[:saltLen], 1000, 2*keyLen+2)
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(keys[2*keyLen:], salt[saltLen:]) {
		return nil, nil, ErrBadPassword
	}
	block, err := aes.NewCipher(keys[:keyLen])
	if err != nil {
		return nil, nil, err
	}
	mac := hmac.New(sha1.New, keys[keyLen:2*keyLen])
	dataLen := size - int64(saltLen) - 2 - 10
	if dataLen < 0 {
		return nil, nil, zip.ErrFormat
	}
	r := &aesCTRReader{r: io.TeeReader(io.LimitReader(raw, dataLen), mac), block: block}
	auth := func() error {
		var code [10]byte
		if _, err := io.ReadFull(raw, code[:]); err != nil {
			return err
		}
		if !hmac.Equal(mac.Sum(nil)[:10], code[:]) {
			return zip.ErrChecksum
		}
		return nil
	}
	return r, auth, nil
}

// aesCTRReader is CTR mode as WinZip does it: the counter is little-endian
// and starts at 1, which crypto/cipher's big-endian CTR cannot express.
type aesCTRReader struct {
	r       io.Reader
	block   cipher.Block
	counter [16]byte
	stream  [16]byte
	used    int
}

func (a *aesCTRReader) Read(b []byte) (int, error) {
	n, err := a.r.Read(b)
	for i := 0; i < n; i++ {
		if a.used == 0 || a.used == 16 {
			for j := range a.counter {
				a.counter[j]++
				if a.counter[j] != 0 {
					break
				}
			}
			a.block.Encrypt(a.stream[:], a.counter[:])
			a.used = 0
		}
		b[i] ^= a.stream[a.used]
		a.used++
	}
	return n, err
}

// modTime is when the member was last modified. A zip that carries only the
// MS-DOS date fields (Explorer's do) records local wall-clock time, which
// archive/zip labels UTC; read it as local time instead.
func modTime(f *zip.File) time.Time {
	t := f.Modified
	if t.IsZero() || hasExtendedTime(f.Extra) {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
}

// hasExtendedTime reports an NTFS (0x000a) or Unix extended (0x5455)
// timestamp field, both of which are true instants.
func hasExtendedTime(extra []byte) bool {
	for len(extra) >= 4 {
		tag := uint16(extra[0]) | uint16(extra[1])<<8
		size := int(uint16(extra[2]) | uint16(extra[3])<<8)
		if tag == 0x000a || tag == 0x5455 {
			return true
		}
		if 4+size > len(extra) {
			break
		}
		extra = extra[4+size:]
	}
	return false
}
