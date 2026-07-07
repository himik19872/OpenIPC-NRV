import React, { useEffect, useState, useCallback } from 'react';
import {
  View, Text, StyleSheet, TouchableOpacity, FlatList, Alert, ActivityIndicator,
} from 'react-native';
import { camerasApi } from '../api/client';

export default function CameraDetailScreen({ route }: any) {
  const { camera } = route.params;
  const [recording, setRecording] = useState(false);
  const [recordings, setRecordings] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  const loadRecordings = useCallback(async () => {
    try {
      const { data } = await camerasApi.getRecordings(camera.id);
      setRecordings(data);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [camera.id]);

  useEffect(() => { loadRecordings(); }, [loadRecordings]);

  const handleStartRecording = async () => {
    try {
      setRecording(true);
      await camerasApi.startRecording(camera.id);
      Alert.alert('Успех', 'Запись запущена');
      loadRecordings();
    } catch {
      Alert.alert('Ошибка', 'Не удалось запустить запись');
    } finally {
      setRecording(false);
    }
  };

  const handleStopRecording = async (recId: string) => {
    try {
      await camerasApi.stopRecording(camera.id, recId);
      Alert.alert('Успех', 'Запись остановлена');
      loadRecordings();
    } catch {
      Alert.alert('Ошибка', 'Не удалось остановить запись');
    }
  };

  return (
    <View style={styles.container}>
      <View style={styles.streamPlaceholder}>
        <Text style={{ color: '#fff', fontSize: 18 }}>
          {camera.is_online ? 'RTSP поток' : 'Нет сигнала'}
        </Text>
        {camera.is_online && (
          <Text style={{ color: '#aaa', fontSize: 12, marginTop: 8 }}>{camera.rtsp_url}</Text>
        )}
      </View>

      <View style={styles.infoRow}>
        <View style={styles.infoItem}>
          <Text style={styles.infoLabel}>RTSP</Text>
          <Text style={styles.infoValue} numberOfLines={1}>{camera.rtsp_url}</Text>
        </View>
        <View style={styles.infoItem}>
          <Text style={styles.infoLabel}>Локация</Text>
          <Text style={styles.infoValue}>{camera.location || '-'}</Text>
        </View>
        <View style={styles.infoItem}>
          <Text style={styles.infoLabel}>Производитель</Text>
          <Text style={styles.infoValue}>{camera.manufacturer || '-'}</Text>
        </View>
      </View>

      <TouchableOpacity
        style={[styles.recordButton, { backgroundColor: recording ? '#ff4d4f' : '#1677ff' }]}
        onPress={handleStartRecording}
        disabled={recording}
      >
        {recording ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={styles.recordButtonText}>Начать запись</Text>
        )}
      </TouchableOpacity>

      <Text style={styles.sectionTitle}>Записи</Text>

      <FlatList
        data={recordings}
        keyExtractor={(item) => item.id}
        renderItem={({ item }) => (
          <View style={styles.recordingItem}>
            <View style={{ flex: 1 }}>
              <Text style={styles.recordingId}>{item.id.slice(0, 8)}...</Text>
              <Text style={styles.recordingDate}>{new Date(item.start_time).toLocaleString()}</Text>
            </View>
            {item.status === 'recording' && (
              <TouchableOpacity onPress={() => handleStopRecording(item.id)}>
                <Text style={{ color: '#ff4d4f', fontWeight: '600' }}>Стоп</Text>
              </TouchableOpacity>
            )}
          </View>
        )}
        ListEmptyComponent={
          <Text style={{ textAlign: 'center', color: '#888', marginTop: 20 }}>Нет записей</Text>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: '#f5f5f5', padding: 16 },
  streamPlaceholder: {
    backgroundColor: '#000', height: 220, borderRadius: 12,
    alignItems: 'center', justifyContent: 'center', marginBottom: 16,
  },
  infoRow: { flexDirection: 'row', gap: 8, marginBottom: 16 },
  infoItem: {
    flex: 1, backgroundColor: '#fff', borderRadius: 10, padding: 12,
    elevation: 1, shadowColor: '#000', shadowOpacity: 0.03, shadowRadius: 2,
  },
  infoLabel: { fontSize: 11, color: '#888', marginBottom: 4 },
  infoValue: { fontSize: 13, fontWeight: '500' },
  recordButton: {
    borderRadius: 10, padding: 16, alignItems: 'center', marginBottom: 20,
  },
  recordButtonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  sectionTitle: { fontSize: 18, fontWeight: '600', marginBottom: 12 },
  recordingItem: {
    flexDirection: 'row', alignItems: 'center', backgroundColor: '#fff',
    borderRadius: 10, padding: 14, marginBottom: 8,
    elevation: 1, shadowColor: '#000', shadowOpacity: 0.03, shadowRadius: 2,
  },
  recordingId: { fontSize: 14, fontWeight: '600' },
  recordingDate: { fontSize: 11, color: '#888', marginTop: 2 },
});