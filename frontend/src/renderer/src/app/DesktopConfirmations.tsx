import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmationDialog } from '@/components/ui/confirmation-dialog'
import { useAppUI } from '@/components/ui/confirm-provider'
import { desktop } from '../api/app'
import type { ConfirmationChoice, ConfirmationEvent } from '../../../shared/contracts/confirmation'

type Request = Extract<ConfirmationEvent, { type: 'open' }>

export function DesktopConfirmations() {
  const { t } = useTranslation('errors')
  const { message } = useAppUI()
  const [requests, setRequests] = useState<Request[]>([])
  const [answeringId, setAnsweringId] = useState<string>()
  const answering = useRef(new Set<string>())

  useEffect(
    () =>
      desktop.confirmations.onEvent((event) => {
        setRequests((current) =>
          event.type === 'open' ? [...current, event] : current.filter(({ id }) => id !== event.id)
        )
      }),
    []
  )

  const request = requests[0]
  if (!request) return null
  const { id, options } = request

  async function answer(choice: ConfirmationChoice) {
    if (answering.current.has(id)) return
    answering.current.add(id)
    setAnsweringId(id)
    try {
      await desktop.confirmations.answer(id, choice)
      setRequests((current) => current.filter((item) => item.id !== id))
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('unexpected'))
    } finally {
      answering.current.delete(id)
      setAnsweringId((current) => (current === id ? undefined : current))
    }
  }

  return (
    <ConfirmationDialog
      key={id}
      title={options.title}
      content={<p className="whitespace-pre-wrap wrap-anywhere">{options.message}</p>}
      okText={options.confirmLabel}
      cancelText={options.cancelLabel}
      danger={options.danger}
      pending={answeringId === id}
      onCancel={() => void answer('cancel')}
      onConfirm={() => void answer('accept')}
      extraAction={
        options.extraLabel
          ? { label: options.extraLabel, onClick: () => void answer('extra') }
          : undefined
      }
    />
  )
}
