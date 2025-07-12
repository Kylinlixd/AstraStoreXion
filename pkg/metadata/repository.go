package metadata

import (
	"context"
	"database/sql"
	"time"
)

// MetadataRepository 元数据存储库接口
type MetadataRepository interface {
	// 保存文件元数据
	SaveFileMetadata(ctx context.Context, metadata *FileMetadata) error

	// 获取文件元数据
	GetFileMetadata(ctx context.Context, fileID string) (*FileMetadata, error)

	// 更新文件元数据
	UpdateFileMetadata(ctx context.Context, metadata *FileMetadata) error

	// 删除文件元数据
	DeleteFileMetadata(ctx context.Context, fileID string) error

	// 更新文件访问统计
	UpdateFileAccessStats(ctx context.Context, fileID string) error

	// 获取热点文件列表
	GetHotFiles(ctx context.Context, limit int) ([]*FileAccessStats, error)
}

// FileAccessStats 文件访问统计
type FileAccessStats struct {
	FileID      string    `json:"file_id"`
	AccessCount int64     `json:"access_count"`
	LastAccess  time.Time `json:"last_access"`
}

// SQLMetadataRepository SQL实现的元数据存储库
type SQLMetadataRepository struct {
	db *sql.DB
}

// NewSQLMetadataRepository 创建一个新的SQL元数据存储库
func NewSQLMetadataRepository(db *sql.DB) MetadataRepository {
	return &SQLMetadataRepository{db: db}
}

// SaveFileMetadata 保存文件元数据
func (r *SQLMetadataRepository) SaveFileMetadata(ctx context.Context, metadata *FileMetadata) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 插入文件元数据
	_, err = tx.ExecContext(ctx,
		"INSERT INTO file_metadata (file_id, shard_key, created_at) VALUES (?, ?, ?)",
		metadata.FileID, metadata.ShardKey, metadata.CreatedAt)
	if err != nil {
		return err
	}

	// 插入块信息
	for _, chunk := range metadata.Chunks {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO chunk_info (file_id, chunk_id, node_id, offset, size) VALUES (?, ?, ?, ?, ?)",
			metadata.FileID, chunk.ChunkID, chunk.NodeID, chunk.Offset, chunk.Size)
		if err != nil {
			return err
		}
	}

	// 初始化文件访问统计
	_, err = tx.ExecContext(ctx,
		"INSERT INTO file_access_stats (file_id, access_count, last_access) VALUES (?, ?, ?)",
		metadata.FileID, 0, time.Now())
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetFileMetadata 获取文件元数据
func (r *SQLMetadataRepository) GetFileMetadata(ctx context.Context, fileID string) (*FileMetadata, error) {
	// 查询文件元数据
	row := r.db.QueryRowContext(ctx,
		"SELECT file_id, shard_key, created_at FROM file_metadata WHERE file_id = ?", fileID)

	var metadata FileMetadata
	err := row.Scan(&metadata.FileID, &metadata.ShardKey, &metadata.CreatedAt)
	if err != nil {
		return nil, err
	}

	// 查询块信息
	rows, err := r.db.QueryContext(ctx,
		"SELECT chunk_id, node_id, offset, size FROM chunk_info WHERE file_id = ?", fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metadata.Chunks = []ChunkInfo{}
	for rows.Next() {
		var chunk ChunkInfo
		err := rows.Scan(&chunk.ChunkID, &chunk.NodeID, &chunk.Offset, &chunk.Size)
		if err != nil {
			return nil, err
		}
		metadata.Chunks = append(metadata.Chunks, chunk)
	}

	return &metadata, nil
}

// UpdateFileMetadata 更新文件元数据
func (r *SQLMetadataRepository) UpdateFileMetadata(ctx context.Context, metadata *FileMetadata) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 更新文件元数据
	_, err = tx.ExecContext(ctx,
		"UPDATE file_metadata SET shard_key = ? WHERE file_id = ?",
		metadata.ShardKey, metadata.FileID)
	if err != nil {
		return err
	}

	// 删除旧的块信息
	_, err = tx.ExecContext(ctx, "DELETE FROM chunk_info WHERE file_id = ?", metadata.FileID)
	if err != nil {
		return err
	}

	// 插入新的块信息
	for _, chunk := range metadata.Chunks {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO chunk_info (file_id, chunk_id, node_id, offset, size) VALUES (?, ?, ?, ?, ?)",
			metadata.FileID, chunk.ChunkID, chunk.NodeID, chunk.Offset, chunk.Size)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteFileMetadata 删除文件元数据
func (r *SQLMetadataRepository) DeleteFileMetadata(ctx context.Context, fileID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 删除文件元数据
	_, err = tx.ExecContext(ctx, "DELETE FROM file_metadata WHERE file_id = ?", fileID)
	if err != nil {
		return err
	}

	// 删除块信息
	_, err = tx.ExecContext(ctx, "DELETE FROM chunk_info WHERE file_id = ?", fileID)
	if err != nil {
		return err
	}

	// 删除文件访问统计
	_, err = tx.ExecContext(ctx, "DELETE FROM file_access_stats WHERE file_id = ?", fileID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateFileAccessStats 更新文件访问统计
func (r *SQLMetadataRepository) UpdateFileAccessStats(ctx context.Context, fileID string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE file_access_stats SET access_count = access_count + 1, last_access = ? WHERE file_id = ?",
		time.Now(), fileID)
	return err
}

// GetHotFiles 获取热点文件列表
func (r *SQLMetadataRepository) GetHotFiles(ctx context.Context, limit int) ([]*FileAccessStats, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT file_id, access_count, last_access FROM file_access_stats ORDER BY access_count DESC LIMIT ?",
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hotFiles := []*FileAccessStats{}
	for rows.Next() {
		var stats FileAccessStats
		err := rows.Scan(&stats.FileID, &stats.AccessCount, &stats.LastAccess)
		if err != nil {
			return nil, err
		}
		hotFiles = append(hotFiles, &stats)
	}

	return hotFiles, nil
}
