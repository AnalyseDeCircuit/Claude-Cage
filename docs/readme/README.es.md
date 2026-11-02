<div align="center">

<h1>Claude Cage</h1>

<p align="center"><strong>Mantenga a la señorita Claude en una jaula para que podamos proteger mejor nuestro tiempo juntos.</strong></p>

<img src="../../assets/claude-cage-character.png" alt="Claude Cage" width="320" />

<p align="center">
  <a href="https://github.com/Lee-xeo/Claude-Cage/releases"><img src="https://img.shields.io/github/v/release/Lee-xeo/Claude-Cage?include_prereleases&amp;style=flat-square&amp;label=release&amp;color=2563eb" alt="GitHub Releases" /></a>
  <a href="https://github.com/Lee-xeo/Claude-Cage/actions/workflows/go.yml"><img src="https://img.shields.io/github/actions/workflow/status/Lee-xeo/Claude-Cage/go.yml?branch=main&amp;style=flat-square&amp;label=checks" alt="GitHub Actions" /></a>
  <a href="#descarga-e-instalación"><img src="https://img.shields.io/badge/macOS-14%2B%20%C2%B7%20Apple%20Silicon-374151?style=flat-square&amp;logo=apple&amp;logoColor=white" alt="macOS 14+ · Apple Silicon" /></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/badge/license-MIT-3b8268?style=flat-square" alt="MIT" /></a>
</p>

**[Sitio web del proyecto](https://lee-xeo.github.io/Claude-Cage/) · [Descargar para macOS](https://github.com/Lee-xeo/Claude-Cage/releases) · [Empezando](../getting-started.md) · [Preguntas frecuentes](../troubleshooting.md) · [Contribuir](../../CONTRIBUTING.md)**

[简体中文](../../README.md) | [繁體中文](README.zh-TW.md) | [Русский](README.ru.md) | [العربية](README.ar.md) | [فارسی](README.fa.md) | [한국어](README.ko.md) | [Español](README.es.md) | [မြန်မာဘာသာ](README.my.md) | [Беларуская](README.be.md) | [پښتو](README.ps.md) | [دری](README.prs.md) | [አማርኛ](README.am.md) | [Soomaali](README.so.md) | [ትግርኛ](README.ti.md) | [Français](README.fr.md)

</div>

## Hartazgo con Anthropic

Como usuarios normales que sólo queremos hablar y colaborar con Claude, estamos cansados de configurar instancias de VPS, perfiles de huellas dactilares del navegador y comprobaciones del entorno, mientras nos preocupamos por las prohibiciones de cuentas y las pérdidas financieras. presentamos **Claude Cage** — **una aplicación de escritorio lista para usar: configure su propio proxy de red y administre su entorno aislado, zona horaria, idioma y comprobaciones de entorno en un solo lugar.**

Claude Cage es una máquina virtual Linux liviana que combina una terminal Linux, un navegador y un editor de archivos en una sola aplicación de escritorio. Sus principales características incluyen:

- Configuraciones de red aisladas del host; Cage enruta todo el tráfico de red dentro de su entorno a través de su propio proxy de red.
- Configuración de huellas digitales del navegador aislada del host, incluida la zona horaria y el idioma, con sincronización automática con la ubicación IP del proxy de red
- Integración en todo el flujo de trabajo de Claude: inicio de sesión en el navegador, autorización de cuenta y proceso de Claude Code
- Las comprobaciones de red y aislamiento integradas pausan el acceso a Internet cuando las comprobaciones críticas fallan o la evidencia está incompleta; Los controles comunitarios y de terceros proporcionan información de referencia adicional.

**Resultados de la verificación ambiental con Cage:**

<table>
  <thead>
    <tr>
      <th width="50%">Comprobación de red y aislamiento</th>
      <th width="50%">Comprobación del navegador de la comunidad</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td align="center" valign="top"><a href="../../assets/network-checks.png"><img src="../../assets/network-checks.png" alt="Comprobación de red y aislamiento" width="400" /></a></td>
      <td align="center" valign="top"><a href="../../assets/community-browser-check.png"><img src="../../assets/community-browser-check.png" alt="Comprobación del navegador de la comunidad" width="400" /></a></td>
    </tr>
  </tbody>
</table>

**Interfaz principal de Cage:**

![Cage · hello-cage](../../www/public/workbench.jpg)

## ¿Qué puedes hacer con la jaula?

- **Trabaja en una sola ventana.** Ejecute Claude Code en la terminal, revise los cambios en el editor y use el navegador Cage para obtener una vista previa de los servicios e iniciar sesión.
- **Controla lo que compartes.** Solo se comparten el espacio de trabajo predeterminado y las carpetas de proyectos abiertas explícitamente. El estado de inicio de sesión de Claude, los perfiles del navegador y las claves SSH del host no se importan automáticamente.
- **Vea el estado de su red.** La administración de proxy de red incorporada le permite importar suscripciones, seleccionar puntos finales y verificar la IP de salida. El entorno permanece fuera de línea hasta que se superan las comprobaciones críticas. Si el proxy de red no está disponible, no se recurre a una conexión directa como alternativa.
- **Continúe la próxima vez.** Las herramientas, los archivos y el estado de inicio de sesión persisten en el disco virtual. Las tareas en segundo plano pueden seguir ejecutándose cuando cierras la ventana.

## Inicio rápido

Envíe esta solicitud a su agente de IA (codex, dsh, workbuddy; probablemente no Claude Code 😁):

```markdown
Ayúdame a instalar https://github.com/Lee-xeo/Claude-Cage y configurar el proxy de red de Cage para usar el punto final xxx en mi instalación local de Clash.
```

Todo lo que necesita es un punto final de proxy de red confiable. Este proyecto no promueve ningún proveedor de proxy; Puede encontrar un proveedor usted mismo, por ejemplo buscando en el sitio web de Black Error.

## Descarga e instalación

La versión inicial admite **Apple Silicon Macs (chips de la serie M), con macOS 14 o posterior**. Actualmente, las plataformas Intel Mac, Windows y Linux no son compatibles con la aplicación de escritorio Cage.

1. Abierto [GitHub Releases](https://github.com/Lee-xeo/Claude-Cage/releases) y seleccione Apple Silicon `.dmg` paquete de instalación en la versión.
2. Abra el paquete de instalación y arrastre **Cage** en **Aplicaciones**; luego ejecútelo desde Aplicaciones.
3. Siga las instrucciones de la aplicación para configurar su propio proxy de red, iniciar el entorno y abrir un proyecto.

El instalador incluye el banco de trabajo y el núcleo del proxy de red. El uso diario no requiere instalar Go, Node.js, Docker o Clash. El primer lanzamiento aún requiere una conexión a Internet para descargar la imagen de Linux y las herramientas de desarrollo. Cage no proporciona un servicio de proxy de red ni una cuenta Claude.

Versión actual: `0.1.0-alpha.1`. El código fuente, el sitio web y los lanzamientos son públicos. Consulte las notas de cada versión para conocer la firma del instalador, la certificación notarial de Apple y el alcance de la validación. La instalación en máquina limpia, la compatibilidad con Mac físicos que ejecutan macOS 14, la autorización de cuentas reales de Claude y las solicitudes de modelos aún requieren una validación completa.

Las versiones con soporte de actualización en la aplicación muestran un control de actualización en la esquina inferior izquierda. Cuando hay una actualización disponible, la información sobre herramientas de la flecha hacia abajo dice "Descargar actualización". Al hacer clic en él, se descarga, verifica, instala y reinicia la aplicación. Guarde y detenga la ejecución de tareas primero. Las actualizaciones se descargan a través del proxy de red seleccionado, sin recurrir a una conexión directa. La instalación automática requiere una firma del mismo editor y verificación de Apple. Las versiones posteriores pueden utilizar actualizaciones dentro de la aplicación. Para más detalles, consulte [Actualizar y desinstalar](../getting-started.md#升级与卸载).

## Comience en tres pasos

### 1. Seleccione un proxy de red

En la página de inicio, haga clic en "Configurar red y proxy" y luego en "Agregar fuente". Importe una suscripción YAML de Clash/Mihomo o un archivo YAML local y seleccione un punto final en "Nodos". Si no hay ningún proxy de red disponible, puede seleccionar explícitamente "Descargar esta suscripción a través de la red de host" o importar un archivo local directamente. Luego, administre los puntos finales en 'Configuración → Red y proxy'.

### 2. Iniciar el entorno

Regrese a la mesa de trabajo y haga clic en "Iniciar entorno Cage". Si la configuración de Git del host necesita protección de seguridad, la aplicación primero explica los efectos. Aplíquelo haciendo clic en "Habilitar protección y continuar". Los compromisos y los pulls para proyectos ordinarios no se ven afectados. Luego, Cage descarga la imagen, instala herramientas y ejecuta comprobaciones de red. Vea el progreso en "Detalles de la operación". Una vez que el entorno esté listo y se pasen las comprobaciones críticas, podrá trabajar en línea. Si falla la verificación, siga las instrucciones para solucionar el problema y vuelva a intentarlo.

### 3. Abra un proyecto y ejecute Claude.

Haga clic en 'Abrir proyecto' para seleccionar una carpeta en su Mac, abra una nueva terminal e ingrese `claude`; luego autorice su propia cuenta en el navegador Cage. También puedes comenzar con el espacio de trabajo predeterminado sin preparar un proyecto. Para conocer los pasos completos, consulte [Empezando](../getting-started.md).

## Comprenda estos límites antes de usarlo

- Las ediciones y eliminaciones en proyectos compartidos afectan directamente a los archivos de su Mac. Utilice control de versiones o copias de seguridad. `.git` y otros metadatos son de sólo lectura de forma predeterminada; realizar confirmaciones de Git en Mac.
- Todos los proyectos comparten un entorno Cage, incluidas sus herramientas, cuentas y sistema de archivos Linux. Los proyectos no están aislados unos de otros.
- Actualmente, el invitado tiene acceso al TCP IPv4 público a través del proxy de red, y el DNS se envía a través del mismo proxy. Se bloquean las conexiones públicas UDP, IPv6, ICMP y iniciadas por invitados al host o a la red local. Es posible que las herramientas que dependen de estas capacidades no funcionen.
- Las comprobaciones de red muestran la configuración actual y pruebas de muestra. No pueden garantizar que una cuenta evite las restricciones de la plataforma y no reemplazan una auditoría completa de seguridad de virtualización.

Para más detalles, consulte [Límites de red y datos](../getting-started.md#网络与数据边界), [Diseño de controles de red.](../design/network-checks.md) y [Diseño de proxy de red integrado](../design/managed-proxy.md).

## Documentación y aportaciones

Para consultar el índice completo, consulte [Índice de documentación](../README.md).

| tu tarea | Empieza aquí |
|---|---|
| Instale, configure un proxy de red y abra su primer proyecto | [Empezando](../getting-started.md) |
| Solucionar problemas de descargas, inicio, conectividad e inicio de sesión | [Preguntas frecuentes y recuperación](../troubleshooting.md) |
| Aprenda los controles del teclado, la apariencia, la persistencia de datos y el comportamiento de salida | [Uso diario](../getting-started.md#日常使用) |
| Construya desde la fuente o contribuya con mejoras | [Guía de contribución](../../CONTRIBUTING.md) |
| Comprender las responsabilidades del módulo y las limitaciones de la red. | [Diseño de banco de trabajo](../design/workbench.md) |
| Informar un problema reproducible | [GitHub Issues](https://github.com/Lee-xeo/Claude-Cage/issues) |

El proyecto está evolucionando rápidamente. Únase al grupo comunitario para compartir comentarios; Los mantenedores de la comunidad son bienvenidos.

<p align="center">
  <a href="../../assets/community-qq.png"><img src="../../assets/community-qq.png" alt="QQ · 1134250870" width="240" /></a>
</p>

## Orígenes y licencias

La jaula se deriva de [agent-vm](https://github.com/sylvinus/agent-vm) ; la línea de base aguas arriba es `652688aae92c35ce536b9be52262e1cc5ec6e03a`.

La licencia principal del proyecto es [MIT](../../LICENSE). Los componentes incluidos conservan sus respectivas licencias. Mihomo se distribuye como un ejecutable independiente bajo GPL-3.0. Ver también [NOTICE](../../NOTICE), [Licencias de terceros](../../THIRD_PARTY_LICENSES), [Fuentes de terceros](../../third_party/SOURCES) y [procedencia noVNC](../../ui/vendor/novnc/README.cage.md). La descripción del proyecto upstream se conserva en el [LÉAME ascendente](../agent-vm-upstream.md).

Cage es un proyecto comunitario independiente y no está afiliado a Anthropic.

## Referencias y agradecimientos

Gracias a los siguientes proyectos por capacidades fundamentales y referencias de diseño:

- [agent-vm](https://github.com/sylvinus/agent-vm): la base aguas arriba de Cage.
- [Lima](https://github.com/lima-vm/lima): Gestión de máquinas virtuales Linux.
- [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock): redes de máquinas virtuales.
- [sshocker](https://github.com/lima-vm/sshocker) y [SFTP](https://github.com/pkg/sftp): intercambio de archivos entre la máquina virtual y el host.
- [noVNC](https://github.com/novnc/noVNC): visualización remota e interacción en el navegador.
- [Mihomo](https://github.com/MetaCubeX/mihomo): el núcleo de proxy de red integrado.
- [code-server](https://github.com/coder/code-server): el editor de código basado en navegador.
- [AIO Sandbox](https://github.com/agent-infra/sandbox): referencias para el ambiente de trabajo y el diseño de interacción.
- [claude-switcher](https://github.com/YOURSJEYO/claude-switcher) y [Claude-Guardian](https://github.com/DUNHKpcc/Claude-Guardian): referencias para comprobaciones de red y diagnósticos de seguridad.
