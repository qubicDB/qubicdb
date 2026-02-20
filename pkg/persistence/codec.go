package persistence

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"

	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/vmihailenco/msgpack/v5"
)

// Binary format constants
const (
	MagicBytes    = "NRDB" // QubicDB magic identifier
	FormatVersion = 1
)

// Header for binary format
type Header struct {
	Magic      [4]byte
	Version    uint16
	Flags      uint16
	IndexIDLen uint32
	DataLen    uint64
	Checksum   uint32
}

const (
	FlagCompressed uint16 = 1 << 0
	FlagEncrypted  uint16 = 1 << 1
)

// Codec handles encoding/decoding of matrices
type Codec struct {
	compress  bool
	compLevel int
}

// NewCodec creates a new codec
func NewCodec(compress bool) *Codec {
	return &Codec{
		compress:  compress,
		compLevel: gzip.BestSpeed, // Fast compression
	}
}

// Encode serializes a matrix to binary format
func (c *Codec) Encode(matrix *core.Matrix) ([]byte, error) {
	// First, encode with msgpack
	data, err := msgpack.Marshal(matrix)
	if err != nil {
		return nil, err
	}

	// Optionally compress
	var flags uint16 = 0
	if c.compress {
		compressed, err := c.compressData(data)
		if err != nil {
			return nil, err
		}
		if len(compressed) < len(data) {
			data = compressed
			flags |= FlagCompressed
		}
	}

	// Build header
	header := Header{
		Version:    FormatVersion,
		Flags:      flags,
		IndexIDLen: uint32(len(matrix.IndexID)),
		DataLen:    uint64(len(data)),
		Checksum:   c.checksum(data),
	}
	copy(header.Magic[:], MagicBytes)

	// Serialize header + indexID + data
	buf := new(bytes.Buffer)

	// Write header
	if err := binary.Write(buf, binary.LittleEndian, header); err != nil {
		return nil, err
	}

	// Write indexID
	if _, err := buf.WriteString(string(matrix.IndexID)); err != nil {
		return nil, err
	}

	// Write data
	if _, err := buf.Write(data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Decode deserializes binary format to a matrix
func (c *Codec) Decode(raw []byte) (*core.Matrix, error) {
	if len(raw) < 24 { // Minimum header size
		return nil, errors.New("data too short")
	}

	buf := bytes.NewReader(raw)

	// Read header
	var header Header
	if err := binary.Read(buf, binary.LittleEndian, &header); err != nil {
		return nil, err
	}

	// Verify magic
	if string(header.Magic[:]) != MagicBytes {
		return nil, errors.New("invalid magic bytes")
	}

	// Check version
	if header.Version > FormatVersion {
		return nil, errors.New("unsupported format version")
	}

	// Read indexID
	indexIDBytes := make([]byte, header.IndexIDLen)
	if _, err := io.ReadFull(buf, indexIDBytes); err != nil {
		return nil, err
	}

	// Read data
	data := make([]byte, header.DataLen)
	if _, err := io.ReadFull(buf, data); err != nil {
		return nil, err
	}

	// Verify checksum
	if c.checksum(data) != header.Checksum {
		return nil, errors.New("checksum mismatch")
	}

	// Decompress if needed
	if header.Flags&FlagCompressed != 0 {
		decompressed, err := c.decompressData(data)
		if err != nil {
			return nil, err
		}
		data = decompressed
	}

	// Decode msgpack
	var matrix core.Matrix
	if err := msgpack.Unmarshal(data, &matrix); err != nil {
		return nil, err
	}

	return &matrix, nil
}

// compressData compresses using gzip
func (c *Codec) compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, c.compLevel)
	if err != nil {
		return nil, err
	}

	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompressData decompresses gzip data
func (c *Codec) decompressData(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// checksum calculates a simple checksum
func (c *Codec) checksum(data []byte) uint32 {
	var sum uint32 = 0
	for i := 0; i < len(data); i++ {
		sum = sum*31 + uint32(data[i])
	}
	return sum
}

// EncodeSnapshot creates a lightweight snapshot for quick persistence
type Snapshot struct {
	IndexID      core.IndexID `msgpack:"index_id"`
	Version      uint64       `msgpack:"version"`
	NeuronCount  int          `msgpack:"neuron_count"`
	SynapseCount int          `msgpack:"synapse_count"`
	CurrentDim   int          `msgpack:"current_dim"`
	TotalEnergy  float64      `msgpack:"total_energy"`
	ModifiedAt   int64        `msgpack:"modified_at"`
}

// CreateSnapshot creates a snapshot from a matrix
func CreateSnapshot(matrix *core.Matrix) Snapshot {
	totalEnergy := 0.0
	for _, n := range matrix.Neurons {
		totalEnergy += n.Energy
	}

	return Snapshot{
		IndexID:      matrix.IndexID,
		Version:      matrix.Version,
		NeuronCount:  len(matrix.Neurons),
		SynapseCount: len(matrix.Synapses),
		CurrentDim:   matrix.CurrentDim,
		TotalEnergy:  totalEnergy,
		ModifiedAt:   matrix.ModifiedAt.Unix(),
	}
}

// EncodeSnapshot serializes a snapshot
func EncodeSnapshot(s Snapshot) ([]byte, error) {
	return msgpack.Marshal(s)
}

// DecodeSnapshot deserializes a snapshot
func DecodeSnapshot(data []byte) (Snapshot, error) {
	var s Snapshot
	err := msgpack.Unmarshal(data, &s)
	return s, err
}
