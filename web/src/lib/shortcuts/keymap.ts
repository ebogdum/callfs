export type ShortcutScope = 'global' | 'lists' | 'data' | 'tables' | 'files' | 'logs' | 'chart' | 'settings';

export type KeyAlternative = string[];

export interface ShortcutDefinition {
	id: string;
	labelKey: string;
	scope: ShortcutScope;
	keys: KeyAlternative[];
}

export type KeymapOverrides = Record<string, KeyAlternative[]>;

const shortcut = (id: string, scope: ShortcutScope, ...keys: KeyAlternative[]): ShortcutDefinition => ({
	id,
	labelKey: `shortcuts.${id}`,
	scope,
	keys
});

export const shortcutDefinitions: readonly ShortcutDefinition[] = [
	shortcut('palette.open', 'global', ['mod', 'k']),
	shortcut('search.focus', 'global', ['/']),
	shortcut('sheet.open', 'global', ['?']),
	shortcut('go.overview', 'global', ['g', 'o']),
	shortcut('go.files', 'global', ['g', 'f']),
	shortcut('go.sharing', 'global', ['g', 's']),
	shortcut('go.cluster', 'global', ['g', 'c']),
	shortcut('go.insights', 'global', ['g', 'i']),
	shortcut('go.activity', 'global', ['g', 'a']),
	shortcut('go.settings', 'global', ['g', ',']),
	shortcut('go.transfers', 'global', ['g', 't']),
	shortcut('go.jobs', 'global', ['g', 'j']),
	shortcut('go.logs', 'global', ['g', 'l']),
	shortcut('sidebar.toggle', 'global', ['[']),
	shortcut('notifications.open', 'global', ['mod', '.']),
	shortcut('theme.toggle', 'global', ['mod', 'shift', 'l']),
	shortcut('escape.close', 'global', ['esc']),
	shortcut('list.next', 'lists', ['j'], ['arrowdown']),
	shortcut('list.previous', 'lists', ['k'], ['arrowup']),
	shortcut('list.open', 'lists', ['enter'], ['o']),
	shortcut('list.select', 'lists', ['x'], ['space']),
	shortcut('list.extendNext', 'lists', ['shift', 'j']),
	shortcut('list.extendPrevious', 'lists', ['shift', 'k']),
	shortcut('list.selectAll', 'lists', ['mod', 'a']),
	shortcut('list.menu', 'lists', ['shift', 'f10'], ['contextmenu']),
	shortcut('data.refresh', 'data', ['r']),
	shortcut('table.export', 'tables', ['e']),
	shortcut('files.up', 'files', ['backspace'], ['alt', 'arrowup']),
	shortcut('files.preview', 'files', ['space']),
	shortcut('files.inspector', 'files', ['i']),
	shortcut('files.rename', 'files', ['f2']),
	shortcut('files.move', 'files', ['m']),
	shortcut('files.download', 'files', ['d']),
	shortcut('files.downloadRange', 'files', ['shift', 'd']),
	shortcut('files.createLink', 'files', ['l']),
	shortcut('files.copyPath', 'files', ['c']),
	shortcut('files.cut', 'files', ['mod', 'x']),
	shortcut('files.paste', 'files', ['mod', 'v']),
	shortcut('files.trash', 'files', ['delete']),
	shortcut('files.deleteForever', 'files', ['shift', 'delete']),
	shortcut('files.upload', 'files', ['u']),
	shortcut('files.uploadFolder', 'files', ['shift', 'u']),
	shortcut('files.newFolder', 'files', ['n']),
	shortcut('files.hidden', 'files', ['.']),
	shortcut('files.view', 'files', ['v']),
	shortcut('files.editPath', 'files', ['mod', 'l']),
	shortcut('logs.liveTail', 'logs', ['t']),
	shortcut('chart.left', 'chart', ['arrowleft']),
	shortcut('chart.right', 'chart', ['arrowright']),
	shortcut('settings.save', 'settings', ['mod', 's'])
];
