<script lang="ts">
    import { onDestroy } from 'svelte';
    import type { ColumnMapping, CSVPreview, Document, ExportFormat, ExportProgress, ExportScope, ImportProgress, ImportValidation, Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import Input from '../../components/Input.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';

    let {
        session,
        document,
        tab = $bindable('export'),
        onclose,
    }: {
        session: Session;
        document: Document;
        tab: 'export' | 'import';
        onclose: () => void;
    } = $props();

    let isTable = $derived(document.kind === 'table');
    let isNativeQuery = $derived(document.kind === 'query' && session.capabilities.nativeQueryExecutor && !session.capabilities.sqlExecutor);

    // ---- Export ----------------------------------------------------------
    let exportFormat = $state<ExportFormat>('csv');
    // svelte-ignore state_referenced_locally -- one-time default seeded from
    // this panel's own document, which never changes for a mounted instance.
    let exportScope = $state<ExportScope>(isTable ? 'full' : 'loaded');
    let exportJobId = $state('');
    let exportProgress = $state<ExportProgress>();
    let exportTimer: ReturnType<typeof setInterval> | undefined;
    const formatExtension: Record<ExportFormat, string> = { csv: 'csv', json: 'json', sql: 'sql' };
    const formatLabel: Record<ExportFormat, string> = {
        csv: 'CSV files',
        json: 'JSON files',
        sql: 'SQL files',
    };
    $effect(() => {
        if (exportScope === 'loaded' && exportFormat === 'sql') exportFormat = 'csv';
    });
    let exportScopeOptions: SelectOption[] = $derived([
        { value: 'loaded', label: 'Loaded rows (already fetched)' },
        ...(isTable ? [{ value: 'filtered', label: 'Filtered rows (current filter)' }] : []),
        ...(!isNativeQuery ? [{ value: 'full', label: isTable ? 'All rows' : 'Full result (re-runs the query)' }] : []),
    ]);
    let exportFormatOptions: SelectOption[] = $derived([
        { value: 'csv', label: 'CSV' },
        { value: 'json', label: 'JSON' },
        ...(isTable && exportScope !== 'loaded' ? [{ value: 'sql', label: 'SQL (INSERT statements)' }] : []),
    ]);
    const importDelimiterOptions: SelectOption[] = [
        { value: ',', label: 'Comma (,)' },
        { value: ';', label: 'Semicolon (;)' },
        { value: '\t', label: 'Tab' },
    ];
    const importEncodingOptions: SelectOption[] = [
        { value: 'auto', label: 'Auto / UTF-8' },
        { value: 'utf-8', label: 'UTF-8' },
        { value: 'utf-16le', label: 'UTF-16 little-endian' },
        { value: 'utf-16be', label: 'UTF-16 big-endian' },
        { value: 'windows-1252', label: 'Windows-1252' },
        { value: 'iso-8859-1', label: 'ISO-8859-1 / Latin-1' },
    ];
    function statementForActiveResult() {
        return document.results[document.activeResult]?.statement || document.sql;
    }
    function toCSV(rows: Record<string, unknown>[]) {
        const columns = document.results[document.activeResult]?.columns?.map((c) => c.name) ?? Object.keys(rows[0] ?? {});
        const cell = (v: unknown) => {
            if (v === null || v === undefined) return '';
            const s = typeof v === 'object' && v !== null && 'base64' in (v as Record<string, unknown>) ? String((v as Record<string, unknown>).base64) : String(v);
            return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
        };
        const lines = [columns.join(',')];
        for (const row of rows) lines.push(columns.map((c) => cell(row[c])).join(','));
        return lines.join('\n');
    }
    async function exportLoadedRows() {
        const rows = document.results[document.activeResult]?.rows ?? [];
        if (!rows.length) {
            workspace.notify('No loaded rows to export yet, run the query first.');
            return;
        }
        const text = exportFormat === 'csv' ? toCSV(rows) : JSON.stringify(rows, null, 2);
        const blob = new Blob([text], { type: 'text/plain' });
        const name = `${document.object || document.title || 'results'}.${formatExtension[exportFormat]}`;
        if (!(await workspace.saveBlobWithPicker(blob, name, formatLabel[exportFormat], formatExtension[exportFormat]))) return;
        workspace.notify(`Exported ${rows.length} loaded rows as ${exportFormat.toUpperCase()}.`);
    }
    async function startBackedExport() {
        const destination = await workspace.chooseExportDestination(
            `${document.object || document.title || 'export'}.${formatExtension[exportFormat]}`,
            formatLabel[exportFormat],
            `*.${formatExtension[exportFormat]}`,
        );
        if (!destination) return;
        exportProgress = undefined;
        const id = await workspace.startExport(session, document, destination, {
            kind: isTable ? 'table' : 'query',
            format: exportFormat,
            scope: exportScope,
            table: document.object,
            filter: document.tableFilter,
            sort: document.sort,
            descending: document.descending,
            sql: statementForActiveResult(),
            delimiter: ',',
        });
        if (!id) return;
        exportJobId = id;
        exportTimer = setInterval(async () => {
            const next = await workspace.exportProgress(id);
            exportProgress = next;
            if (next.done) {
                clearInterval(exportTimer);
                exportTimer = undefined;
            }
        }, 250);
    }
    function runExport() {
        if (exportScope === 'loaded') exportLoadedRows();
        else startBackedExport();
    }
    function cancelExport() {
        if (exportJobId && !exportProgress?.done) workspace.cancelBackgroundJob(exportJobId);
    }
    function resetExport() {
        exportJobId = '';
        exportProgress = undefined;
    }

    // ---- Import ------------------------------------------------------------
    let importStep = $state<'pick' | 'preview' | 'apply'>('pick');
    let importPath = $state('');
    let importDelimiter = $state(',');
    let importEncoding = $state('auto');
    let importHasHeader = $state(true);
    let importNullToken = $state('');
    let importPreview = $state<CSVPreview>();
    let importMapping = $state<Record<number, string>>({});
    let importJobId = $state('');
    let importProgress = $state<ImportProgress>();
    let importValidation = $state<ImportValidation>();
    let importValidationKey = $state('');
    let importValidating = $state(false);
    let importTimer: ReturnType<typeof setInterval> | undefined;
    let table = $derived(session.schema.find((t) => t.name === document.object));
    let mappedColumns = $derived(new Set(Object.values(importMapping).filter(Boolean)));
    let mappingOptions: SelectOption[] = $derived([
        { value: '', label: 'Skip this column' },
        ...(table?.columns ?? []).map((column) => ({
            value: column.name,
            label: `${column.name}${column.primaryKey ? ' (primary key)' : ''}`,
        })),
    ]);
    let unmappedRequired = $derived((table?.columns ?? []).filter((c) => c.notNull && !c.defaultValue && !c.primaryKey && !mappedColumns.has(c.name)));
    let currentValidationKey = $derived(
        JSON.stringify({ path: importPath, delimiter: importDelimiter, encoding: importEncoding, hasHeader: importHasHeader, nullToken: importNullToken, mapping: importMapping }),
    );
    let validationCurrent = $derived(!!importValidation && importValidationKey === currentValidationKey);
    async function chooseImportFile() {
        const chosen = await workspace.chooseImportFile();
        if (!chosen) return;
        importPath = chosen;
        await refreshImportPreview();
    }
    async function refreshImportPreview() {
        if (!importPath) return;
        const result = await workspace.previewImport(importPath, importDelimiter, importEncoding, importHasHeader);
        if (!result) return;
        importPreview = result;
        const columns = table?.columns ?? [];
        const next: Record<number, string> = {};
        result.columns.forEach((name, i) => {
            const match = columns.find((c) => c.name.toLowerCase() === name.toLowerCase());
            if (match) next[i] = match.name;
        });
        importMapping = next;
        importValidation = undefined;
        importValidationKey = '';
        importStep = 'preview';
    }
    function importOptions() {
        return { delimiter: importDelimiter, encoding: importEncoding, hasHeader: importHasHeader, nullToken: importNullToken, batchSize: 0 };
    }
    function mappedEntries(): ColumnMapping[] {
        return Object.entries(importMapping)
            .filter(([, column]) => column)
            .map(([csvColumn, column]) => ({ csvColumn: Number(csvColumn), column }));
    }
    async function validateImport() {
        const entries = mappedEntries();
        if (!entries.length) {
            workspace.notify('Map at least one CSV column to a table column before validating.');
            return;
        }
        importValidating = true;
        const key = currentValidationKey;
        const result = await workspace.validateImport(session, importPath, document.object, entries, importOptions());
        importValidating = false;
        if (result && key === currentValidationKey) {
            importValidation = result;
            importValidationKey = key;
        }
    }
    function startImportApply() {
        const entries = mappedEntries();
        if (!entries.length) {
            workspace.notify('Map at least one CSV column to a table column before importing.');
            return;
        }
        if (!validationCurrent || importValidation?.invalidRows) {
            workspace.notify('Validate the current import plan and fix invalid rows before importing.');
            return;
        }
        importStep = 'apply';
        workspace.startImport(session, document, importPath, document.object, entries, importOptions()).then((id) => {
            if (!id) {
                importStep = 'preview';
                return;
            }
            importJobId = id;
            importTimer = setInterval(async () => {
                const next = await workspace.importProgress(id);
                importProgress = next;
                if (next.done) {
                    clearInterval(importTimer);
                    importTimer = undefined;
                    if (!next.fatalError) workspace.runTableQuery(session, document);
                }
            }, 250);
        });
    }
    function cancelImport() {
        if (importJobId && !importProgress?.done) workspace.cancelBackgroundJob(importJobId);
    }
    function resetImport() {
        importStep = 'pick';
        importPath = '';
        importPreview = undefined;
        importJobId = '';
        importProgress = undefined;
        importValidation = undefined;
        importValidationKey = '';
    }

    onDestroy(() => {
        clearInterval(exportTimer);
        clearInterval(importTimer);
    });
</script>

<svelte:window
    onkeydown={(event) => {
        if (event.key === 'Escape') onclose();
    }}
/>

<aside class="row-inspector transfer-panel" aria-label="Import and export">
    <header>
        <div>
            <span class="eyebrow">DATA TRANSFER</span>
            <h3>{document.title}</h3>
            <small>Export results, or import a CSV into this table.</small>
        </div>
        <Button icon="close" label="Close data transfer panel" onclick={onclose} />
    </header>
    <div class="segmented row-detail-tabs" role="tablist" aria-label="Transfer mode">
        <button role="tab" aria-selected={tab === 'export'} class:active={tab === 'export'} onclick={() => (tab = 'export')}> Export </button>
        {#if isTable}<button role="tab" aria-selected={tab === 'import'} class:active={tab === 'import'} onclick={() => (tab = 'import')}> Import </button>{/if}
    </div>

    {#if tab === 'export'}
        <div class="panel-body">
            {#if !exportJobId}
                <Select bind:value={exportScope} options={exportScopeOptions} label="Scope" />
                <Select bind:value={exportFormat} options={exportFormatOptions} label="Format" />
                <p class="panel-hint">
                    <Icon name="disconnect" size={13} />
                    {#if exportScope === 'loaded'}
                        Downloads exactly what's already loaded in this grid.
                    {:else if exportScope === 'full' && !isTable}
                        Bounded like interactive execution: at most 10,000 rows.
                    {:else}
                        Streams directly to a file you choose. Stays off the UI thread.
                    {/if}
                </p>
                <Button variant="primary" icon="download" onclick={runExport}>
                    {exportScope === 'loaded' ? 'Export' : 'Choose destination & export'}
                </Button>
            {:else}
                <div class="panel-progress">
                    {#if !exportProgress?.done}
                        <span class="spinner"></span>
                        <span>Exporting… {exportProgress?.rows ?? 0} rows written</span>
                        <Button variant="outline" onclick={cancelExport}>Cancel</Button>
                    {:else if exportProgress.error}
                        <Icon name="disconnect" />
                        <span class="error-text">{exportProgress.error}</span>
                        <Button variant="outline" onclick={resetExport}>Back</Button>
                    {:else}
                        <Icon name="check" />
                        <span>Exported {exportProgress.rows} rows.</span>
                        <Button variant="outline" onclick={resetExport}>Export again</Button>
                    {/if}
                </div>
            {/if}
        </div>
    {:else}
        <div class="panel-body">
            {#if importStep === 'pick'}
                <p class="panel-hint">Choose a CSV file to insert into this table.</p>
                <Select bind:value={importDelimiter} options={importDelimiterOptions} label="Delimiter" />
                <Select bind:value={importEncoding} options={importEncodingOptions} label="Source encoding" />
                <Input type="checkbox" bind:checked={importHasHeader} label="First row is a header" />
                <Button variant="primary" icon="link" onclick={chooseImportFile}>Choose CSV file…</Button>
            {:else if importStep === 'preview' && importPreview}
                <p class="panel-hint">
                    {importPreview.totalRows}{importPreview.totalRowsCapped ? '+' : ''} data row{importPreview.totalRows === 1 ? '' : 's'} found.
                    <button class="text-button" onclick={resetImport}>Change file</button>
                </p>
                <label class="panel-field">
                    <span>Treat this text as NULL</span>
                    <input bind:value={importNullToken} placeholder="(empty string)" />
                </label>
                <div class="mapping-list">
                    {#each importPreview.columns as name, i}<div class="mapping-row">
                            <div class="mapping-source">
                                <strong>{name}</strong>
                                <small>{importPreview.sample[0]?.[i] ?? ''}</small>
                            </div>
                            <Select bind:value={importMapping[i]} options={mappingOptions} placeholder={`Map ${name} to`} compact />
                        </div>{/each}
                </div>
                {#if unmappedRequired.length}<p class="panel-warning">
                        <Icon name="disconnect" size={13} />
                        Not-null column{unmappedRequired.length === 1 ? '' : 's'}
                        {unmappedRequired.map((c) => c.name).join(', ')}
                        {unmappedRequired.length === 1 ? 'is' : 'are'} unmapped; those rows will fail.
                    </p>{/if}
                {#if !table?.columns.some((c) => c.primaryKey)}<p class="panel-warning">
                        <Icon name="disconnect" size={13} />
                        This table has no primary key, so it can't be imported into yet.
                    </p>{/if}
                {#if validationCurrent && importValidation}<div class="import-validation" class:invalid={importValidation.invalidRows > 0}>
                        <strong>{importValidation.validRows} valid · {importValidation.invalidRows} invalid</strong>
                        {#if importValidation.uncheckedColumns.length}<small>
                                Database-specific types for {importValidation.uncheckedColumns.join(', ')} will still be checked by the database.
                            </small>{/if}
                        {#if importValidation.issues.length}<ul class="panel-errors">
                                {#each importValidation.issues as issue}<li>
                                        Row {issue.row}, {issue.csvColumn} → {issue.column} ({issue.expected}): {issue.message}
                                    </li>{/each}
                            </ul>{/if}
                    </div>{/if}
                <Button variant="outline" icon="check" disabled={importValidating || !table?.columns.some((c) => c.primaryKey)} onclick={validateImport}>
                    {importValidating ? 'Validating…' : validationCurrent ? 'Validate again' : 'Validate data'}
                </Button>
                <Button
                    variant="primary"
                    icon="download"
                    disabled={!table?.columns.some((c) => c.primaryKey) || !validationCurrent || !!importValidation?.invalidRows}
                    onclick={startImportApply}
                >
                    Import {importPreview.totalRows}{importPreview.totalRowsCapped ? '+' : ''} rows
                </Button>
            {:else if importStep === 'apply'}
                <div class="panel-progress">
                    {#if !importProgress?.done}
                        <span class="spinner"></span>
                        <span>
                            Importing… {importProgress?.imported ?? 0} imported, {importProgress?.failed ?? 0} failed
                        </span>
                        <Button variant="outline" onclick={cancelImport}>Cancel</Button>
                    {:else}
                        <Icon name={importProgress.fatalError ? 'disconnect' : 'check'} />
                        <span class:error-text={!!importProgress.fatalError}>
                            {importProgress.fatalError ||
                                `Imported ${importProgress.imported} row${importProgress.imported === 1 ? '' : 's'}${importProgress.failed ? `; ${importProgress.failed} failed` : ''}.`}
                        </span>
                        <Button variant="outline" onclick={resetImport}>Import another file</Button>
                    {/if}
                    {#if importProgress?.errors?.length}<ul class="panel-errors">
                            {#each importProgress.errors as error}<li>{error}</li>{/each}
                        </ul>{/if}
                </div>
            {/if}
        </div>
    {/if}
</aside>

<style>
    .panel-body {
        display: flex;
        flex-direction: column;
        gap: 14px;
        padding: 16px;
        overflow-y: auto;
    }
    .panel-field {
        display: flex;
        flex-direction: column;
        gap: 6px;
        font-size: 13px;
    }
    .panel-hint {
        display: flex;
        align-items: flex-start;
        gap: 6px;
        margin: 0;
        color: var(--muted);
        font-size: 12px;
    }
    .panel-warning {
        display: flex;
        align-items: flex-start;
        gap: 6px;
        margin: 0;
        color: var(--error);
        font-size: 12px;
    }
    .panel-progress {
        display: flex;
        flex-direction: column;
        align-items: flex-start;
        gap: 10px;
    }
    .import-validation {
        display: grid;
        gap: 6px;
        padding: 10px;
        border: 1px solid var(--success);
        border-radius: 6px;
        background: var(--success-bg);
        font-size: 12px;
    }
    .import-validation.invalid {
        border-color: var(--error);
        background: color-mix(in srgb, var(--error) 10%, var(--surface));
    }
    .import-validation small {
        color: var(--muted);
    }
    .panel-errors {
        margin: 0;
        padding-left: 18px;
        font-size: 12px;
        color: var(--error);
        max-height: 140px;
        overflow: auto;
    }
    .mapping-list {
        display: flex;
        flex-direction: column;
        gap: 8px;
        max-height: 320px;
        overflow-y: auto;
        padding-right: 2px;
    }
    .mapping-row {
        display: flex;
        flex-direction: column;
        gap: 6px;
        padding: 8px 10px;
        border: 1px solid var(--border-soft);
        border-radius: 6px;
        background: var(--subtle);
    }
    .mapping-source {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: 8px;
        font-size: 12px;
    }
    .mapping-source strong {
        color: var(--text);
    }
    .mapping-source small {
        color: var(--muted);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        max-width: 140px;
    }
</style>
