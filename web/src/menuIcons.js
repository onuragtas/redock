/**
 * Backend'den gelen icon key'leri (örn. "mdiHome") ile @mdi/js path'lerini eşleştirir.
 * Menü verisi API'den gelir; frontend sadece route + bu icon map'i tutar.
 */
import {
  mdiAccountGroup,
  mdiBugCheck,
  mdiCloud,
  mdiConsole,
  mdiDns,
  mdiDocker,
  mdiEmail,
  mdiDownload,
  mdiHome,
  mdiLan,
  mdiLanConnect,
  mdiLaptop,
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
  mdiNetworkOutline,
  mdiDns,
  mdiServerNetwork,
  mdiEmail,
  mdiCloud,
  mdiLan,
  mdiConsole,
  mdiLanConnect,
  mdiWeb,
  mdiScriptText,
  mdiBugCheck,
  mdiDownload,
  mdiAccountGroup
}

export default menuIconMap
