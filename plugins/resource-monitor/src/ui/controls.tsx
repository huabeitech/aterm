export function SelectField({
  value,
  options,
  disabled,
  onValueChange,
  'aria-label': label
}: {
  value: string
  options: { value: string; label: string }[]
  disabled?: boolean
  onValueChange: (value: string) => void
  'aria-label': string
}) {
  return (
    <select
      aria-label={label}
      value={value}
      disabled={disabled}
      onChange={(event) => onValueChange(event.target.value)}
    >
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  )
}
