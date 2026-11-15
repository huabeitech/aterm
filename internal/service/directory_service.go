package service

import "context"

type DirectoryService struct {
	store HostStore
}

func NewDirectoryService(store HostStore) *DirectoryService {
	return &DirectoryService{store: store}
}

func (s *DirectoryService) ListDirectories(ctx context.Context) ([]Directory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.store.ListDirectories()
}

func (s *DirectoryService) CreateDirectory(ctx context.Context, input DirectoryInput) (Directory, error) {
	if err := ctx.Err(); err != nil {
		return Directory{}, err
	}
	input = NormalizeDirectoryInput(input)
	if err := ValidateDirectoryInput(input); err != nil {
		return Directory{}, err
	}
	return s.store.CreateDirectory(input)
}

func (s *DirectoryService) UpdateDirectory(ctx context.Context, id string, input DirectoryInput) (Directory, error) {
	if err := ctx.Err(); err != nil {
		return Directory{}, err
	}
	input = NormalizeDirectoryInput(input)
	if err := ValidateDirectoryInput(input); err != nil {
		return Directory{}, err
	}
	return s.store.UpdateDirectory(id, input)
}

func (s *DirectoryService) MoveDirectory(ctx context.Context, id string, input DirectoryMoveInput) (Directory, error) {
	if err := ctx.Err(); err != nil {
		return Directory{}, err
	}
	return s.store.MoveDirectory(id, input)
}

func (s *DirectoryService) DeleteDirectory(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ensureDirectoryEmpty(id); err != nil {
		return err
	}
	return s.store.DeleteDirectory(id)
}

func (s *DirectoryService) ensureDirectoryEmpty(id string) error {
	directories, err := s.store.ListDirectories()
	if err != nil {
		return err
	}
	directoryIDs := directoryDescendantIDs(directories, id)
	hosts, err := s.store.ListHosts()
	if err != nil {
		return err
	}
	for _, host := range hosts {
		if directoryIDs[host.DirectoryID] {
			return ErrDirectoryNotEmpty
		}
	}
	return nil
}

func directoryDescendantIDs(directories []Directory, id string) map[string]bool {
	ids := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, directory := range directories {
			if ids[directory.ID] || !ids[directory.ParentID] {
				continue
			}
			ids[directory.ID], changed = true, true
		}
	}
	return ids
}
