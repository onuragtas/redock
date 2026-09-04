/**
 * Backend'den gelen icon key'leri (örn. "mdiHome") ile @mdi/js path'lerini eşleştirir.
 * Menü verisi API'den gelir; frontend sadece route + bu icon map'i tutar.
 */
import {
  mdiAccountGroup,
  mdiArchive,
  mdiBellOutline,
  mdiBugCheck,
  mdiCloud,
  mdiConsole,
  mdiCubeOutline,
  mdiDns,
  mdiDocker,
  mdiEmail,
  mdiDownload,
  mdiHome,
  mdiIncognito,
  mdiLan,
  mdiLanConnect,
  mdiLaptop,
  mdiMemory,
  mdiNetworkOutline,
  mdiRocket,
  mdiScriptText,
  mdiServerNetwork,
  mdiWeb,
  mdiWrench
} from '@mdi/js'

export const menuIconMap = {
  mdiHome,
  mdiRocket,
  mdiWrench,
  mdiLaptop,
  mdiDocker,
  mdiCubeOutline,
  mdiNetworkOutline,
  mdiDns,
  mdiServerNetwork,
  mdiEmail,
  mdiCloud,
  mdiIncognito,
  mdiLan,
  mdiConsole,
  mdiLanConnect,
  mdiWeb,
  mdiScriptText,
  mdiBugCheck,
  mdiDownload,
  mdiAccountGroup,
  mdiArchive,
  mdiBellOutline,
  mdiMemory
}

/**
 * An icon name the map does not know used to be handed to <path d> as-is, which
 * the browser reports as "Expected number" and draws as nothing. Falling back to
 * a real icon keeps the menu usable and the console quiet while the missing
 * entry is added.
 */
export function menuIcon(name) {
  return menuIconMap[name] || mdiCubeOutline
}

export default menuIconMap
