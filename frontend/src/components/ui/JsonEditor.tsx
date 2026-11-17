import { useEffect, useMemo, useRef } from 'react'
import { EditorState, Compartment } from '@codemirror/state'
import { EditorView, keymap, lineNumbers } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { json, jsonParseLinter } from '@codemirror/lang-json'
import { linter, lintGutter } from '@codemirror/lint'
import { cn } from '@/lib/utils'

export interface JsonEditorProps {
  value: string
  onChange?: (next: string) => void
  readOnly?: boolean
  ariaLabel?: string
  minHeight?: string
  maxHeight?: string
  className?: string
}

const canMountCodeMirror =
  typeof window !== 'undefined' &&
  typeof document !== 'undefined' &&
  typeof Range !== 'undefined' &&
  typeof Range.prototype.getClientRects === 'function'

function buildTheme(minHeight: string, maxHeight?: string) {
  return EditorView.theme({
    '&': {
      backgroundColor: 'var(--code-bg)',
      color: 'var(--code-text)',
      fontFamily: 'var(--font-mono)',
      fontSize: '12.5px',
      borderRadius: '0.375rem',
      border: '1px solid var(--border)',
    },
    '&.cm-focused': {
      outline: 'none',
      borderColor: 'var(--accent)',
      boxShadow: '0 0 0 3px var(--accent-ring)',
    },
    '.cm-scroller': {
      fontFamily: 'var(--font-mono)',
      lineHeight: '1.55',
      minHeight,
      ...(maxHeight ? { maxHeight, overflow: 'auto' } : {}),
    },
    '.cm-content': {
      caretColor: 'var(--code-text)',
      padding: '10px 0',
    },
    '.cm-line': {
      padding: '0 12px',
    },
    '.cm-gutters': {
      backgroundColor: 'var(--code-bg)',
      color: 'var(--text-faint)',
      borderRight: '1px solid var(--border)',
      borderTopLeftRadius: '0.375rem',
      borderBottomLeftRadius: '0.375rem',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'transparent',
      color: 'var(--code-text)',
    },
    '.cm-selectionBackground, ::selection': {
      backgroundColor: 'var(--accent-ring) !important',
    },
    '.cm-diagnostic-error': {
      borderLeft: '3px solid var(--danger)',
    },
    '.cm-tooltip': {
      backgroundColor: 'var(--panel-strong)',
      color: 'var(--text)',
      border: '1px solid var(--border-strong)',
    },
  })
}

export function JsonEditor({
  value,
  onChange,
  readOnly = false,
  ariaLabel = 'JSON 编辑器',
  minHeight = '220px',
  maxHeight = '520px',
  className,
}: JsonEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  const readOnlyComp = useRef(new Compartment())

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  const syntaxError = useMemo(() => {
    const trimmed = value.trim()
    if (!trimmed) return 'JSON 内容不能为空'
    try {
      JSON.parse(trimmed)
      return ''
    } catch (e) {
      return (e as Error).message
    }
  }, [value])

  useEffect(() => {
    if (!canMountCodeMirror || !hostRef.current) return

    const state = EditorState.create({
      doc: value,
      extensions: [
        lineNumbers(),
        history(),
        keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap]),
        json(),
        lintGutter(),
        linter(jsonParseLinter()),
        buildTheme(minHeight, maxHeight),
        EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
        readOnlyComp.current.of([
          EditorState.readOnly.of(readOnly),
          EditorView.editable.of(!readOnly),
        ]),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChangeRef.current?.(update.state.doc.toString())
          }
        }),
      ],
    })

    const view = new EditorView({
      state,
      parent: hostRef.current,
    })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
    // Only create EditorView on mount / dimension change; value updates sync via transaction below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ariaLabel, minHeight, maxHeight])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    view.dispatch({
      effects: readOnlyComp.current.reconfigure([
        EditorState.readOnly.of(readOnly),
        EditorView.editable.of(!readOnly),
      ]),
    })
  }, [readOnly])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    const current = view.state.doc.toString()
    if (current !== value) {
      view.dispatch({
        changes: { from: 0, to: current.length, insert: value },
      })
    }
  }, [value])

  return (
    <div className={cn('space-y-1.5', className)}>
      {canMountCodeMirror ? (
        <div ref={hostRef} />
      ) : (
        <textarea
          aria-label={ariaLabel}
          readOnly={readOnly}
          value={value}
          onChange={(e) => onChange?.(e.target.value)}
          style={{ minHeight, maxHeight }}
          className="w-full rounded-md border border-[var(--border)] bg-[var(--code-bg)] p-3 font-mono text-xs leading-5 text-[var(--code-text)] focus:border-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
        />
      )}
      {syntaxError && (
        <p className="font-mono text-[11px] text-[var(--danger)]">
          JSON 语法错误：{syntaxError}
        </p>
      )}
    </div>
  )
}
