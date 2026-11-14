import { Check } from 'lucide-react'
import { PALETTES, type ColorMode, type ThemeId } from '../lib/themes'

export function ThemePicker({ mode, value, onChange }: { mode: ColorMode; value: ThemeId; onChange: (id: ThemeId) => void }) {
  const label = mode === 'light' ? 'Default light theme' : 'Default dark theme'
  return <fieldset className="theme-picker">
    <legend>{label}</legend>
    <div className="theme-options">
      {PALETTES.map(palette => {
        const colors = palette.colors[mode]
        return <label className="theme-option" data-selected={value === palette.id} key={palette.id}>
          <input className="sr-only" type="radio" name={mode + '-palette'} aria-label={palette.name} value={palette.id} checked={value === palette.id} onChange={() => onChange(palette.id)} />
          <span className="theme-swatches" aria-hidden="true">
            {[colors.bg, colors.side, colors.panel, colors.accent].map((color, index) => <span key={index} style={{ background: color }} />)}
          </span>
          <span className="theme-option-label">{palette.name}{value === palette.id && <Check size={13} aria-hidden="true" />}</span>
        </label>
      })}
    </div>
  </fieldset>
}
