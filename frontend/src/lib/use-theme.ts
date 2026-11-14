import { useLayoutEffect, useState } from 'react'
import { getPalette, resolveMode, type Appearance, type ThemeId } from './themes'

export function useTheme(appearance: Appearance, lightTheme: ThemeId, darkTheme: ThemeId) {
  const [mode, setMode] = useState(() => resolveMode(appearance, matchMedia('(prefers-color-scheme: dark)').matches))
  useLayoutEffect(() => {
    const media = matchMedia('(prefers-color-scheme: dark)')
    function apply() {
      const resolved = resolveMode(appearance, media.matches)
      const palette = getPalette(resolved === 'dark' ? darkTheme : lightTheme)
      const root = document.documentElement
      root.classList.toggle('dark', resolved === 'dark')
      root.dataset.palette = palette.id
      root.style.colorScheme = resolved
      for (const [token, color] of Object.entries(palette.colors[resolved])) {
        root.style.setProperty('--' + token, color)
      }
      setMode(resolved)
    }
    apply()
    media.addEventListener('change', apply)
    return () => media.removeEventListener('change', apply)
  }, [appearance, lightTheme, darkTheme])
  return mode
}
