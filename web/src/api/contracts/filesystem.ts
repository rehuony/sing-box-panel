import type { ListFilesystemEntriesData, ResolveFilesystemPathData } from '../generated';

export type { FilesystemEntry, FilesystemMode, FilesystemPage, FilesystemSelection } from '../generated';
export type FilesystemQuery = NonNullable<ListFilesystemEntriesData['query']>;
export type FilesystemResolveInput = NonNullable<ResolveFilesystemPathData['query']>;
