import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

export function GridBackground({
  className,
  children,
  fadeClassName,
}: {
  className?: string;
  children?: ReactNode;
  fadeClassName?: string;
}) {
  return (
    <div
      aria-hidden={children ? undefined : true}
      className={cn('relative overflow-hidden', className)}
    >
      <div
        className={cn(
          'pointer-events-none absolute inset-0 opacity-70',
          '[background-size:36px_36px]',
          '[background-image:linear-gradient(to_right,rgba(20,24,31,0.07)_1px,transparent_1px),linear-gradient(to_bottom,rgba(20,24,31,0.07)_1px,transparent_1px)]',
        )}
      />
      <div
        className={cn(
          'pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top,rgba(40,86,255,0.16),transparent_34%),radial-gradient(circle_at_bottom_right,rgba(191,138,67,0.12),transparent_26%),linear-gradient(180deg,rgba(255,253,248,0.9),rgba(255,253,248,0.72))]',
          fadeClassName,
        )}
      />
      {children ? <div className="relative z-10">{children}</div> : null}
    </div>
  );
}
