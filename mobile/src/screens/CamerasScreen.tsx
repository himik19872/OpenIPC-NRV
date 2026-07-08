import React, { useEffect, useState } from 'react';
import { View, Text, StyleSheet, FlatList, TouchableOpacity, RefreshControl } from 'react-native';
import { camerasApi } from '../api/client';

export default function CamerasScreen({ navigation }: any) {
  const [cameras, setCameras] = useState<any[]>([]);
  const [refreshing, setRefreshing] = useState(false);

  const load = async () => {
    try {
      const { data } = await camerasApi.list();
      setCameras(data);
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => { load(); }, []);

  const onRefresh = async () => {
    setRefreshing(true);
    await load();
    setRefreshing(false);
  };

  return (
    <View style={styles.container}>
      <Text style={styles.title}>Камеры</Text>

      <FlatList
        data={cameras}
        keyExtractor={(item) => item.id}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} />}
        renderItem={({ item }) => (
          <TouchableOpacity
            style={styles.cameraCard}
            onPress={() => navigation.navigate('CameraDetail', { camera: item })}
          >
            <View style={{ flex: 1 }}>
              <Text style={styles.cameraName}>{item.name}</Text>
              <Text style={styles.cameraUrl} numberOfLines={1}>{item.rtsp_main_url}</Text>
              <Text style={styles.cameraLocation}>{item.location || '-'}</Text>
            </View>
            <View style={[styles.statusBadge, { backgroundColor: item.is_online ? '#52c41a' : '#ff4d4f' }]}>
              <Text style={styles.statusText}>{item.is_online ? 'ON' : 'OFF'}</Text>
            </View>
          </TouchableOpacity>
        )}
        ListEmptyComponent={
          <Text style={{ textAlign: 'center', color: '#888', marginTop: 40 }}>Нет добавленных камер</Text>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: '#f5f5f5', padding: 16 },
  title: { fontSize: 24, fontWeight: '700', marginBottom: 16 },
  cameraCard: {
    flexDirection: 'row', alignItems: 'center', backgroundColor: '#fff',
    borderRadius: 12, padding: 16, marginBottom: 10,
    elevation: 2, shadowColor: '#000', shadowOpacity: 0.05, shadowRadius: 4,
  },
  cameraName: { fontSize: 16, fontWeight: '600', marginBottom: 4 },
  cameraUrl: { fontSize: 11, color: '#aaa' },
  cameraLocation: { fontSize: 12, color: '#888', marginTop: 4 },
  statusBadge: {
    borderRadius: 20, paddingHorizontal: 12, paddingVertical: 4,
    marginLeft: 8,
  },
  statusText: { color: '#fff', fontWeight: '700', fontSize: 12 },
});