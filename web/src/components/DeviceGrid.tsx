import { AnimatePresence, motion } from 'motion/react'

import { DeviceCard } from '@/components/DeviceCard'
import type { DeviceState } from '@/types/contract'

interface DeviceGridProps {
  devices: DeviceState[]
}

/** Responsive card grid. Cards animate in/out and reflow as devices change. */
export function DeviceGrid({ devices }: DeviceGridProps) {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <AnimatePresence mode="popLayout" initial={false}>
        {devices.map((device) => (
          <motion.div
            key={device.device_id}
            layout
            className="h-full"
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.97 }}
            transition={{ duration: 0.2 }}
          >
            <DeviceCard device={device} />
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  )
}
