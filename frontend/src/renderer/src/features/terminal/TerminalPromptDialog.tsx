import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmationDialog } from '@/components/ui/confirmation-dialog'
import { useAppUI } from '@/components/ui/confirm-provider'
import { PasswordInput } from '@/components/ui/desktop'
import { TextField } from '@/components/ui/workbench-controls'
import type { TerminalPrompt } from '../../../../shared/contracts/terminal'
import { desktop } from '../../api/app'

export function TerminalPromptDialog({ prompt }: { prompt: TerminalPrompt }) {
  const { t } = useTranslation('terminal')
  const { message } = useAppUI()
  const [answers, setAnswers] = useState(() => prompt.fields.map(() => ''))
  const [pending, setPending] = useState(false)
  const answering = useRef(false)

  async function answer(accepted: boolean) {
    if (answering.current) return
    answering.current = true
    setPending(true)
    try {
      await desktop.terminal.answer(prompt.id, accepted ? answers : null)
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('unableToOpenTerminal'))
      answering.current = false
      setPending(false)
    }
  }

  const actionLabel = {
    'host-key': t('trustAndConnect'),
    'host-key-changed': t('replaceKeyAndConnect'),
    credentials: t('continue')
  }[prompt.kind]

  return (
    <ConfirmationDialog
      title={prompt.title}
      okText={actionLabel}
      submitOnEnter={prompt.kind === 'credentials'}
      pending={pending}
      onConfirm={() => void answer(true)}
      onCancel={() => void answer(false)}
      content={
        <div className="min-w-0 [&_[data-slot=input-group-addon]]:mr-0">
          <p className="whitespace-pre-wrap wrap-anywhere leading-[1.6]">{prompt.message}</p>
          {prompt.fields.map((field, index) => {
            const Input = field.secret ? PasswordInput : TextField
            return (
              <label
                key={index}
                className="mt-3.5 grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5 p-1"
              >
                <span className="wrap-anywhere">{field.label}</span>
                <Input
                  autoFocus={index === 0}
                  autoComplete="off"
                  value={answers[index]}
                  onChange={(event) => {
                    const value = event.target.value
                    setAnswers((current) => current.map((answer, i) => (i === index ? value : answer)))
                  }}
                />
              </label>
            )
          })}
        </div>
      }
    />
  )
}
