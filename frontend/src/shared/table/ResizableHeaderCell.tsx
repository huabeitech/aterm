import type {CSSProperties, ReactNode, ThHTMLAttributes} from 'react';

type Props = ThHTMLAttributes<HTMLTableCellElement> & {
    width?: number;
    minWidth?: number;
    maxWidth?: number;
    onResize?: (width: number) => void;
    children?: ReactNode;
};

export function ResizableHeaderCell({width, minWidth = 80, maxWidth = 640, onResize, children, style, ...props}: Props) {
    function startResize(event: React.MouseEvent<HTMLSpanElement>) {
        if (!width || !onResize) return;
        event.preventDefault();
        event.stopPropagation();
        const startX = event.clientX;
        const startWidth = width;
        const handleMouseMove = (moveEvent: MouseEvent) => onResize(clamp(startWidth + moveEvent.clientX - startX, minWidth, maxWidth));
        const handleMouseUp = () => {
            document.body.classList.remove('is-resizing-table-column');
            window.removeEventListener('mousemove', handleMouseMove);
            window.removeEventListener('mouseup', handleMouseUp);
        };
        document.body.classList.add('is-resizing-table-column');
        window.addEventListener('mousemove', handleMouseMove);
        window.addEventListener('mouseup', handleMouseUp);
    }

    const mergedStyle: CSSProperties = width ? {...style, width} : style ?? {};
    return <th {...props} style={mergedStyle}>{children}{onResize && <span className="table-column-resizer" onMouseDown={startResize}/>}</th>;
}

function clamp(value: number, min: number, max: number) {
    return Math.min(Math.max(value, min), max);
}
