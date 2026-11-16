import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { AlertDialog } from 'radix-ui'
import { AlertCircle, Layers, Trash2, X } from 'lucide-react'
import { Button, CopyButton, ScrollArea, Section, ListSkeleton } from './components'
import type { Translate } from './i18n'
import type { useImages } from './useImages'
import type { Engine } from './useDocker'
type Images = ReturnType<typeof useImages>
function size(bytes: number) {
  if (!bytes) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB'],
    index = Math.min(3, Math.floor(Math.log(bytes) / Math.log(1024)))
  return `${(bytes / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}
export function ImagesPane({
  model,
  t,
  locale,
  engine,
  session,
  style,
  separator,
  visible,
  query,
  filter
}: {
  model: Images
  t: Translate
  locale: string
  engine: Engine | null
  session: string
  style: CSSProperties
  separator: ReactNode
  visible: boolean
  query: string
  filter: string
}) {
  const [confirm, setConfirm] = useState(false)
  useEffect(() => {
    if (!visible || !model.selected) setConfirm(false)
  }, [visible, model.selected?.ID])
  const rows = model.images.filter(
    (image) =>
      (image.RepoTags.join(' ') + ' ' + image.ID).toLowerCase().includes(query.toLowerCase()) &&
      (filter === 'all' || !image.RepoTags.length)
  )
  const selected = model.selected,
    detail = model.detail,
    name = selected?.RepoTags[0] || t('untagged')
  return (
    <>
      <section id="images-list" className="container-list images-list" aria-label={t('imagesTab')}>
        <ScrollArea className="list-area">
          {!model.ready ? (
            <div className="list-empty">
              {model.busy || !model.error ? (
                <ListSkeleton label={t('loading')} />
              ) : (
                <>
                  <Layers />
                  <p>{model.error}</p>
                  <Button variant="outline" onClick={() => void model.refresh()}>
                    {t('retry')}
                  </Button>
                </>
              )}
            </div>
          ) : !rows.length ? (
            <div className="list-empty">
              <Layers />
              <h2 className="title-4">{t(model.images.length ? 'imageNoMatch' : 'imageEmpty')}</h2>
              <p className="caption-5 muted">
                {t(model.images.length ? 'noMatchHelp' : 'imageEmptyHelp')}
              </p>
            </div>
          ) : (
            <table className="image-table">
              <thead>
                <tr>
                  <th className="caption-5">{t('imageName')}</th>
                  <th className="image-usage caption-5">{t('imageUsage')}</th>
                  <th className="image-size caption-5">{t('imageSize')}</th>
                  <th className="image-created caption-5">{t('imageCreated')}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((image) => (
                  <tr
                    key={image.ID}
                    tabIndex={0}
                    aria-selected={selected?.ID === image.ID}
                    onClick={() => model.choose(image.ID)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        model.choose(image.ID)
                      }
                    }}
                  >
                    <td>
                      <div className="container-name title-5">
                        <Layers />
                        <span title={image.RepoTags.join('\n')}>
                          {image.RepoTags[0] || t('untagged')}
                        </span>
                      </div>
                      <div className="container-id caption-5 muted" title={image.ID}>
                        {image.ID.replace(/^sha256:/, '').slice(0, 12)}
                        {image.RepoTags.length > 1 ? ` · +${image.RepoTags.length - 1}` : ''}
                      </div>
                    </td>
                    <td className="image-usage">
                      <span
                        className="status caption-5"
                        data-state={image.Containers > 0 ? 'running' : 'exited'}
                        title={t('imageUsageHelp', { count: image.Containers })}
                      >
                        <i aria-hidden="true" />
                        {image.Containers > 0
                          ? t('imageInUse', { count: image.Containers })
                          : t('imageUnused')}
                      </span>
                    </td>
                    <td className="image-size caption-5 muted">{image.Size || '—'}</td>
                    <td className="image-created caption-5 muted" title={image.CreatedAt}>
                      {image.CreatedAt || '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </ScrollArea>
        <footer className="list-footer caption-5" role="status">
          {model.busy ? t('busy') : `${rows.length} / ${model.images.length} ${t('imageCount')}`}
        </footer>
      </section>
      {selected && (
        <>
          {separator}
          <aside id="image-detail" className="detail" style={style} aria-label={t('imageDetails')}>
            <header className="detail-heading">
              <div className="detail-identity">
                <div className="detail-name">
                  <Layers />
                  <h2 className="title-4" title={name}>
                    {name}
                  </h2>
                </div>
                <p className="detail-image caption-5 muted">
                  {selected.ID.replace(/^sha256:/, '').slice(0, 12)}
                </p>
              </div>
              <Button
                icon
                title={t('back')}
                aria-label={t('back')}
                onClick={() => model.choose(null)}
              >
                <X />
              </Button>
            </header>
            <div className="image-detail-actions">
              <Button
                variant="outline"
                disabled={!model.active || model.busy}
                onClick={() => setConfirm(true)}
              >
                <Trash2 />
                {t('imageRemove')}
              </Button>
            </div>
            <ScrollArea className="detail-area" viewportClassName="detail-scroll">
              {model.detailError ? (
                <div className="detail-error" role="alert">
                  <AlertCircle />
                  <p>{model.detailError}</p>
                  <Button variant="outline" onClick={() => model.choose(selected.ID)}>
                    {t('retry')}
                  </Button>
                </div>
              ) : detail ? (
                <>
                  <Section title={t('summary')}>
                    <dl className="properties caption-5">
                      <div>
                        <dt>{t('imageId')}</dt>
                        <dd className="with-copy">
                          <code title={detail.Id}>
                            {detail.Id.replace(/^sha256:/, '').slice(0, 12)}
                          </code>
                          <CopyButton value={detail.Id} label={t('copyId')} />
                        </dd>
                      </div>
                      <div>
                        <dt>{t('imageSize')}</dt>
                        <dd>{size(detail.Size)}</dd>
                      </div>
                      <div>
                        <dt>{t('imagePlatform')}</dt>
                        <dd>
                          {detail.Os} / {detail.Architecture}
                        </dd>
                      </div>
                      <div>
                        <dt>{t('imageLayers')}</dt>
                        <dd>{detail.Layers}</dd>
                      </div>
                      <div>
                        <dt>{t('imageCreated')}</dt>
                        <dd>
                          {detail.Created && !Number.isNaN(Date.parse(detail.Created))
                            ? new Date(detail.Created).toLocaleString(locale)
                            : '—'}
                        </dd>
                      </div>
                    </dl>
                  </Section>
                  <Section title={t('imageTags')} count={detail.RepoTags.length}>
                    <div className="network-list">
                      {detail.RepoTags.length ? (
                        detail.RepoTags.map((tag) => (
                          <span className="network-chip caption-5" key={tag}>
                            {tag}
                          </span>
                        ))
                      ) : (
                        <span className="caption-5 muted">{t('untagged')}</span>
                      )}
                    </div>
                  </Section>
                  <Section title={t('imageCommand')}>
                    <dl className="properties caption-5">
                      <div>
                        <dt>{t('imageEntrypoint')}</dt>
                        <dd>
                          <code>{detail.Entrypoint.join(' ') || '—'}</code>
                        </dd>
                      </div>
                      <div>
                        <dt>{t('imageCommand')}</dt>
                        <dd>
                          <code>{detail.Cmd.join(' ') || '—'}</code>
                        </dd>
                      </div>
                    </dl>
                  </Section>
                  {!!detail.RepoDigests.length && (
                    <Section title={t('imageDigest')}>
                      <div className="digest-list caption-5">
                        {detail.RepoDigests.map((d) => (
                          <code key={d}>{d}</code>
                        ))}
                      </div>
                    </Section>
                  )}
                </>
              ) : (
                <div className="detail-skeleton" aria-busy="true">
                  {[1, 2, 3, 4].map((i) => (
                    <div className="skeleton" key={i} />
                  ))}
                </div>
              )}
            </ScrollArea>
          </aside>
        </>
      )}
      <AlertDialog.Root open={confirm} onOpenChange={setConfirm}>
        <AlertDialog.Portal>
          <AlertDialog.Overlay className="dialog-overlay" />
          <AlertDialog.Content className="dialog">
            <div className="dialog-symbol danger">
              <Trash2 />
            </div>
            <AlertDialog.Title className="title-4">
              {t('imageRemove')} · {name}
            </AlertDialog.Title>
            <AlertDialog.Description className="body-6 muted">
              {t('imageRemoveText', { name, host: `${session} / ${engine?.name ?? ''}` })}
            </AlertDialog.Description>
            <div className="dialog-actions">
              <AlertDialog.Cancel asChild>
                <Button variant="outline">{t('cancel')}</Button>
              </AlertDialog.Cancel>
              <AlertDialog.Action asChild>
                <Button variant="destructive" onClick={() => void model.remove()}>
                  {t('imageRemove')}
                </Button>
              </AlertDialog.Action>
            </div>
          </AlertDialog.Content>
        </AlertDialog.Portal>
      </AlertDialog.Root>
    </>
  )
}
