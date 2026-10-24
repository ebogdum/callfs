export type Tone = 'neutral' | 'accent' | 'ok' | 'warn' | 'danger' | 'info';
export type Size = 'sm' | 'md' | 'lg';
export type Density = 'compact' | 'default' | 'comfortable';
export type GapToken = '1' | '1-5' | '2' | '3' | '4' | '5' | '6' | '8' | '10' | '12';
export type IconName = keyof typeof import('./quarks/icons').icons;
export type StatusKey = import('$lib/status/catalog').StatusKey;
export interface MenuItem { id: string; label: string; icon?: IconName; shortcut?: string[]; tone?: 'default' | 'danger'; disabledReason?: string; group?: string; checked?: boolean; onselect: () => void; children?: MenuItem[] }
export interface ColumnDef<T> { id: string; header: string; cell: import('svelte').Snippet<[T]>; sortKey?: string; align?: 'start' | 'end' | 'center'; width?: number; minWidth?: number; hidden?: boolean; pinned?: boolean; priority?: 1 | 2 | 3; csv?: (row: T) => string; headerHelp?: string }
export interface SortState { key: string; dir: 'asc' | 'desc' }
export interface Selection { mode: 'none' | 'single' | 'multi'; ids: Set<string>; allMatching: boolean }
export interface UiError { title: string; body: string; status?: number; code?: string; requestId?: string; retryable: boolean; layer?: 'network' | 'auth' | 'permission' | 'not_found' | 'conflict' | 'locked' | 'precondition' | 'limit' | 'peer' | 'server' | 'unknown'; fixAction?: { label: string; href?: string; run?: () => void } }
export type QueryStatus = 'loading' | 'empty' | 'error' | 'partial' | 'success';
export interface Series { id: string; label: string; unit: Unit; points: [number, number | null][]; dashed?: boolean; colorIndex?: number }
export type Unit = 'bytes' | 'bytes_per_s' | 'count' | 'per_s' | 'ms' | 's' | 'percent' | 'ratio' | 'none';
export interface Threshold { value: number; tone: 'warn' | 'danger'; label: string }
export interface Annotation { at: number; label: string; tone: Tone; href?: string }
export interface Facet { key: string; label: string; values: { value: string; label: string; count?: number }[] }
export interface Chip { facet: string; op: ':' | '>' | '<' | '>=' | '<=' | '!'; value: string; label: string }
