import { useEffect, useState } from 'react';
import { Typography } from '@mui/material';

export default function StorageSettings({language}:{language:'es'|'en'}){
  const [runtime,setRuntime]=useState<{storageType?:string;dbPath?:string}|null>(null);
  useEffect(()=>{window.cashflow?.runtime?.().then(setRuntime).catch(()=>setRuntime(null));},[]);
  return <details className="settings-section" open><summary><span><Typography component="h2" variant="h5">{language==='es'?'Almacenamiento':'Storage'}</Typography><Typography color="text.secondary">{language==='es'?'Base de datos local utilizada por la aplicación.':'Local database used by the application.'}</Typography></span><span className="settings-chevron" aria-hidden="true">⌄</span></summary><div className="settings-section-content"><Typography>{runtime?.storageType==='local_sqlite'?'SQLite local':'SQLite'}</Typography>{runtime?.dbPath&&<Typography variant="body2" color="text.secondary">{language==='es'?'Ruta de la base de datos: ':'Database path: '}<code className="storage-path">{runtime.dbPath}</code></Typography>}</div></details>;
}
