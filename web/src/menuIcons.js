/**
 * Backend'den gelen icon key'leri (örn. "mdiHome") ile @mdi/js path'lerini eşleştirir.
 * Menü verisi API'den gelir; frontend sadece route + bu icon map'i tutar.
 */
import {
  mdiAccountGroup,
  mdiArchive,
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
  mdiMemory
}

export default menuIconMap
