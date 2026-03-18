import React from 'react';
import { motion, type MotionProps } from 'motion/react';

import { cn } from '@/lib/utils';

const animationProps: MotionProps = {
  initial: { '--x': '100%', scale: 0.98 },
  animate: { '--x': '-100%', scale: 1 },
  whileTap: { scale: 0.95 },
  transition: {
    repeat: Infinity,
    repeatType: 'loop',
    repeatDelay: 1,
    type: 'spring',
    stiffness: 20,
    damping: 15,
    mass: 2,
    scale: {
      type: 'spring',
      stiffness: 200,
      damping: 5,
      mass: 0.5,
    },
  },
};

interface ShinyButtonProps
  extends
    Omit<React.HTMLAttributes<HTMLElement>, keyof MotionProps>,
    MotionProps {
  children: React.ReactNode;
  className?: string;
}

export const ShinyButton = React.forwardRef<
  HTMLButtonElement,
  ShinyButtonProps
>(({ children, className, ...props }, ref) => {
  return (
    <motion.button
      ref={ref}
      className={cn(
        'relative inline-flex cursor-pointer items-center justify-center rounded-full border border-[color:rgba(40,86,255,0.18)] bg-[linear-gradient(180deg,rgba(255,255,255,0.96),rgba(232,239,255,0.92))] px-5 py-2.5 font-semibold text-[var(--color-primary)] shadow-[0_18px_40px_rgba(40,86,255,0.14)] backdrop-blur-xl transition-shadow duration-300 ease-in-out hover:shadow-[0_20px_46px_rgba(40,86,255,0.2)]',
        className,
      )}
      {...animationProps}
      {...props}
    >
      <span
        className="relative block size-full text-sm tracking-[0.08em] uppercase"
        style={{
          maskImage:
            'linear-gradient(-75deg,rgba(40,86,255,0.5) calc(var(--x) + 20%),transparent calc(var(--x) + 30%),rgba(40,86,255,1) calc(var(--x) + 100%))',
        }}
      >
        {children}
      </span>
      <span
        style={{
          mask: 'linear-gradient(rgb(0,0,0), rgb(0,0,0)) content-box exclude,linear-gradient(rgb(0,0,0), rgb(0,0,0))',
          WebkitMask:
            'linear-gradient(rgb(0,0,0), rgb(0,0,0)) content-box exclude,linear-gradient(rgb(0,0,0), rgb(0,0,0))',
          backgroundImage:
            'linear-gradient(-75deg,rgba(40,86,255,0.08) calc(var(--x) + 20%),rgba(40,86,255,0.42) calc(var(--x) + 25%),rgba(40,86,255,0.08) calc(var(--x) + 100%))',
        }}
        className="absolute inset-0 z-10 block rounded-[inherit] p-px"
      />
    </motion.button>
  );
});

ShinyButton.displayName = 'ShinyButton';
